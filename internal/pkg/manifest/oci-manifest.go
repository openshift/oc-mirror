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
	"strings"

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
