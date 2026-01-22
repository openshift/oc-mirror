package manifest

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	digest "github.com/opencontainers/go-digest"
	"github.com/otiai10/copy"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	tarutils "github.com/openshift/oc-mirror/v2/internal/pkg/archive/utils"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/parser"
)

type Manifest struct {
	Log clog.PluggableLoggerInterface
}

func New(log clog.PluggableLoggerInterface) *Manifest {
	return &Manifest{Log: log}
}

// GetOCIImageIndex - used to get the oci index.json
func (o Manifest) GetOCIImageIndex(dir string) (*v2alpha1.OCISchema, error) {
	indexPath := filepath.Join(dir, index)
	oci, err := o.GetOCIImageManifest(indexPath)
	if err != nil {
		return nil, fmt.Errorf("image index %q: %w", indexPath, err)
	}
	return oci, nil
}

// GetOCIImageManifest used to ge the manifest in the oci blobs/sha256
// directory - found in index.json
func (o Manifest) GetOCIImageManifest(file string) (*v2alpha1.OCISchema, error) {
	oci, err := parser.ParseJsonFile[*v2alpha1.OCISchema](file)
	if err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	return oci, nil
}

// GetOperatorConfig used to parse the operator json
func (o Manifest) GetOperatorConfig(file string) (*v2alpha1.OperatorConfigSchema, error) {
	ocs, err := parser.ParseJsonFile[*v2alpha1.OperatorConfigSchema](file)
	if err != nil {
		return nil, fmt.Errorf("operator config: %w", err)
	}
	return ocs, nil
}

// GetOCIImageFromIndex recurses through an index image to find an OCI image.
func (o Manifest) GetOCIImageFromIndex(dir string) (gcrv1.Image, error) { //nolint:ireturn // interface type is required by go-containerregistry
	ociIdx, err := layout.ImageIndexFromPath(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read oci index image: %w", err)
	}

	// This is for now an arbitrary limit. The max we've seen so far is 3 (for catalogs):
	// oci on disk index -> catalog manifest list index -> archful image
	const maxRecurseDepth = 5
	return getImageFromIndex(ociIdx, maxRecurseDepth)
}

func getImageFromIndex(idx gcrv1.ImageIndex, maxDepth uint8) (gcrv1.Image, error) { //nolint:ireturn // interface type is required by go-containerregistry
	errNoImgFound := fmt.Errorf("no image found in oci index")

	// We have reached max recursion depth, give up.
	if maxDepth == 0 {
		return nil, errNoImgFound
	}

	idxDigest, err := idx.Digest()
	if err != nil {
		return nil, fmt.Errorf("failed to get oci index digest: %w", err)
	}

	idxManifest, err := idx.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("failed to read oci index %q manifest: %w", idxDigest.String(), err)
	}

	if len(idxManifest.Manifests) == 0 {
		return nil, fmt.Errorf("no manifests found for oci index %q", idxDigest.String())
	}

	// If we find any image in the Index's manifests, return it right away.
	if imgPos := slices.IndexFunc(idxManifest.Manifests, func(d gcrv1.Descriptor) bool {
		return d.MediaType.IsImage()
	}); imgPos != -1 {
		dgest := idxManifest.Manifests[imgPos].Digest
		img, err := idx.Image(dgest)
		if err != nil {
			return nil, fmt.Errorf("failed to get image %q from oci index %q: %w", dgest.String(), idxDigest.String(), err)
		}
		return img, nil
	}

	// Keep looking for an image in the first of the OCI indexes contained in the current index.
	if idxPos := slices.IndexFunc(idxManifest.Manifests, func(d gcrv1.Descriptor) bool {
		return d.MediaType.IsIndex()
	}); idxPos != -1 {
		desc := idxManifest.Manifests[idxPos]
		childIdx, err := idx.ImageIndex(desc.Digest)
		if err != nil {
			return nil, fmt.Errorf("failed to get image index %q from index %q: %w", desc.Digest.String(), idxDigest.String(), err)
		}
		return getImageFromIndex(childIdx, maxDepth-1)
	}

	return nil, errNoImgFound
}

// ExtractOCILayers
func (o Manifest) ExtractOCILayers(fromPath, toPath, label string, oci *v2alpha1.OCISchema) error {
	_, err := os.Stat(filepath.Join(toPath, label))
	if err == nil {
		o.Log.Debug("extract directory exists (nop)")
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("extract directory: %w", err)
	}
	// Remove any separators in label
	label = strings.Trim(label, "/")
	for _, blob := range oci.Layers {
		validDigest, err := digest.Parse(blob.Digest)
		if err != nil {
			return fmt.Errorf("digest %q: format is not correct: %w", blob.Digest, err)
		}
		digestString := validDigest.Encoded()
		f, err := os.Open(filepath.Join(fromPath, digestString))
		if err != nil {
			return fmt.Errorf("digest %q: open origin layer: %w", digestString, err)
		}
		if err := untargz(f, toPath, label); err != nil {
			return fmt.Errorf("untar %q: %w", digestString, err)
		}
	}
	return nil
}

func untargz(f *os.File, destDir string, label string) error {
	uncompressedStream, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("untar: gzipStream - %w", err)
	}
	return tarutils.UntarWithFilter(uncompressedStream, destDir, func(header *tar.Header) bool {
		return strings.Contains(header.Name, label)
	})
}

// GetReleaseSchema
func (o Manifest) GetReleaseSchema(filePath string) ([]v2alpha1.RelatedImage, error) {
	release, err := parser.ParseJsonFile[v2alpha1.ReleaseSchema](filePath)
	if err != nil {
		return nil, fmt.Errorf("release schema: %w", err)
	}

	allImages := make([]v2alpha1.RelatedImage, 0, len(release.Spec.Tags))
	for _, item := range release.Spec.Tags {
		allImages = append(allImages, v2alpha1.RelatedImage{
			Image: item.From.Name,
			Name:  item.Name,
			Type:  v2alpha1.TypeOCPReleaseContent,
		})
	}
	return allImages, nil
}

// ConvertIndex converts the index.json to a single manifest which refers to a multi manifest index in the blobs/sha256 directory
// this is necessary because containers/image does not support multi manifest indexes on the top level folder
func (o Manifest) ConvertOCIIndexToSingleManifest(dir string, oci *v2alpha1.OCISchema) error {
	data, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		return fmt.Errorf("read index.json: %w", err)
	}
	hash := sha256.Sum256(data)
	digest := hex.EncodeToString(hash[:])
	size := len(data)
	o.Log.Debug("Digest:", digest)
	o.Log.Debug("Size:", size)

	err = copy.Copy(filepath.Join(dir, "index.json"), filepath.Join(dir, "blobs", "sha256", digest))
	if err != nil {
		return fmt.Errorf("copy index.json to destination: %w", err)
	}

	idx := v2alpha1.OCISchema{
		SchemaVersion: oci.SchemaVersion,
		Manifests: []v2alpha1.OCIManifest{
			{
				MediaType: oci.MediaType,
				Digest:    fmt.Sprintf("sha256:%s", digest),
				Size:      size,
			},
		},
	}

	idxData, err := json.Marshal(idx)
	if err != nil {
		return fmt.Errorf("encode manifest OCISchema: %w", err)
	}

	// Write the JSON string to a file
	err = os.WriteFile(filepath.Join(dir, "index.json"), idxData, 0644) // nolint:gosec // G306: no sensitive data
	if err != nil {
		return fmt.Errorf("write single manifest index.json: %w", err)
	}

	return nil
}
