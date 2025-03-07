package manifest

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	digest "github.com/opencontainers/go-digest"
	"github.com/otiai10/copy"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/parser"
)

type Manifest struct {
	Log clog.PluggableLoggerInterface
}

func New(log clog.PluggableLoggerInterface) *Manifest {
	return &Manifest{Log: log}
}

// GetImageIndex - used to get the oci index.json
func (o Manifest) GetImageIndex(dir string) (*v2alpha1.OCISchema, error) {
	indexPath := filepath.Join(dir, index)
	oci, err := o.GetImageManifest(indexPath)
	if err != nil {
		return nil, fmt.Errorf("image index %q: %w", indexPath, err)
	}
	return oci, nil
}

// GetImageManifest used to ge the manifest in the oci blobs/sha256
// directory - found in index.json
func (o Manifest) GetImageManifest(file string) (*v2alpha1.OCISchema, error) {
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

// ExtractLayersOCI
func (o Manifest) ExtractLayersOCI(fromPath, toPath, label string, oci *v2alpha1.OCISchema) error {
	_, err := os.Stat(filepath.Join(toPath, label))
	if err == nil {
		o.Log.Debug("extract directory exists (nop)")
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("extract directory: %w", err)
	}
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
		if err := untar(f, toPath, label); err != nil {
			return fmt.Errorf("untar %q: %w", digestString, err)
		}
	}
	return nil
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

// UntarLayers simple function that untars the image layers
func untar(gzipStream io.Reader, path string, cfgDirName string) error {
	// Remove any separators in cfgDirName as received from the label
	cfgDirName = strings.TrimSuffix(cfgDirName, "/")
	cfgDirName = strings.TrimPrefix(cfgDirName, "/")
	uncompressedStream, err := gzip.NewReader(gzipStream)
	if err != nil {
		return fmt.Errorf("untar: gzipStream - %w", err)
	}

	tarReader := tar.NewReader(uncompressedStream)
	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf("untar: Next() failed: %s", err.Error())
		}

		if strings.Contains(header.Name, cfgDirName) {
			switch header.Typeflag {
			case tar.TypeDir:
				if header.Name != "./" {
					if err := os.MkdirAll(filepath.Join(path, header.Name), 0755); err != nil {
						return fmt.Errorf("untar: Mkdir() failed: %w", err)
					}
				}
			case tar.TypeReg:
				err := os.MkdirAll(filepath.Dir(filepath.Join(path, header.Name)), 0755)
				if err != nil {
					return fmt.Errorf("untar: Create() failed: %w", err)
				}
				outFile, err := os.Create(filepath.Join(path, header.Name))
				if err != nil {
					return fmt.Errorf("untar: Create() failed: %w", err)
				}
				if _, err := io.Copy(outFile, tarReader); err != nil {
					outFile.Close()
					return fmt.Errorf("untar: Copy() failed: %w", err)
				}
				outFile.Close()

			default:
				// just ignore errors as we are only interested in the FB configs layer
			}
		}
	}
	return nil
}

// ConvertIndex converts the index.json to a single manifest which refers to a multi manifest index in the blobs/sha256 directory
// this is necessary because containers/image does not support multi manifest indexes on the top level folder
func (o Manifest) ConvertIndexToSingleManifest(dir string, oci *v2alpha1.OCISchema) error {
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

func (o Manifest) GetDigest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string) (string, error) {
	if err := mirror.ReexecIfNecessaryForImages(imgRef); err != nil {
		return "", fmt.Errorf("reexec mirror: %w", err)
	}

	srcRef, err := alltransports.ParseImageName(imgRef)
	if err != nil {
		return "", fmt.Errorf("invalid source name %s: %w", imgRef, err)
	}

	img, err := srcRef.NewImageSource(ctx, sourceCtx)
	if err != nil {
		return "", fmt.Errorf("new image source: %w", err)
	}
	defer img.Close()

	manifestBytes, _, err := img.GetManifest(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("get manifest: %w", err)
	}

	digest, err := manifest.Digest(manifestBytes)
	if err != nil {
		return "", fmt.Errorf("get manifest digest: %w", err)
	}

	return digest.Encoded(), nil
}
