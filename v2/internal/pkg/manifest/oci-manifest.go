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
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	digest "github.com/opencontainers/go-digest"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/otiai10/copy"
)

var internalLog clog.PluggableLoggerInterface

type Manifest struct {
	Log clog.PluggableLoggerInterface
}

func New(log clog.PluggableLoggerInterface) ManifestInterface {
	internalLog = log
	return &Manifest{Log: log}
}

// GetImageIndex - used to get the oci index.json
func (o Manifest) GetImageIndex(dir string) (*v2alpha1.OCISchema, error) {
	setInternalLog(o.Log)
	var oci *v2alpha1.OCISchema
	indx, err := os.ReadFile(dir + "/" + index)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(indx, &oci)
	if err != nil {
		return nil, err
	}
	return oci, nil
}

// GetImageManifest used to ge the manifest in the oci blobs/sha254
// directory - found in index.json
func (o Manifest) GetImageManifest(file string) (*v2alpha1.OCISchema, error) {
	setInternalLog(o.Log)
	var oci *v2alpha1.OCISchema
	manifest, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(manifest, &oci)
	if err != nil {
		return nil, err
	}
	return oci, nil
}

// GetOperatorConfig used to parse the operator json
func (o Manifest) GetOperatorConfig(file string) (*v2alpha1.OperatorConfigSchema, error) {
	setInternalLog(o.Log)
	var ocs *v2alpha1.OperatorConfigSchema
	manifest, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(manifest, &ocs)
	if err != nil {
		return nil, err
	}
	return ocs, nil
}

// ExtractLayersOCI
func (o Manifest) ExtractLayersOCI(fromPath, toPath, label string, oci *v2alpha1.OCISchema) error {
	setInternalLog(o.Log)
	if _, err := os.Stat(toPath + "/" + label); errors.Is(err, os.ErrNotExist) {
		for _, blob := range oci.Layers {
			validDigest, err := digest.Parse(blob.Digest)
			if err != nil {
				return fmt.Errorf("the digest format is not correct %s ", blob.Digest)
			}
			f, err := os.Open(fromPath + "/" + validDigest.Encoded())
			if err != nil {
				return err
			}
			err = untar(f, toPath, label)
			if err != nil {
				return err
			}
		}
	} else {
		o.Log.Debug("extract directory exists (nop)")
	}
	return nil
}

// GetReleaseSchema
func (o Manifest) GetReleaseSchema(filePath string) ([]v2alpha1.RelatedImage, error) {
	setInternalLog(o.Log)
	var release = v2alpha1.ReleaseSchema{}

	file, err := os.ReadFile(filePath)
	if err != nil {
		return []v2alpha1.RelatedImage{}, err
	}

	err = json.Unmarshal([]byte(file), &release)
	if err != nil {
		return []v2alpha1.RelatedImage{}, err
	}

	var allImages []v2alpha1.RelatedImage
	for _, item := range release.Spec.Tags {
		allImages = append(allImages, v2alpha1.RelatedImage{Image: item.From.Name, Name: item.Name, Type: v2alpha1.TypeOCPReleaseContent})
	}
	return allImages, nil
}

// UntarLayers simple function that untars the image layers
func untar(gzipStream io.Reader, path string, cfgDirName string) error {
	//Remove any separators in cfgDirName as received from the label
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
						return fmt.Errorf("untar: Mkdir() failed: %v", err)
					}
				}
			case tar.TypeReg:
				err := os.MkdirAll(filepath.Dir(filepath.Join(path, header.Name)), 0755)
				if err != nil {
					return fmt.Errorf("untar: Create() failed: %v", err)
				}
				outFile, err := os.Create(filepath.Join(path, header.Name))
				if err != nil {
					return fmt.Errorf("untar: Create() failed: %v", err)
				}
				if _, err := io.Copy(outFile, tarReader); err != nil {
					return fmt.Errorf("untar: Copy() failed: %v", err)
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
	setInternalLog(o.Log)

	data, err := os.ReadFile(path.Join(dir, "index.json"))
	if err != nil {
		o.Log.Debug(err.Error())
	}
	hash := sha256.Sum256(data)
	digest := hex.EncodeToString(hash[:])
	size := len(data)
	log.Println("Digest:", digest)
	log.Println("Size:", size)

	err = copy.Copy(path.Join(dir, "index.json"), path.Join(dir, "blobs", "sha256", digest))
	if err != nil {
		return err
	}

	idx := v2alpha1.OCISchema{
		SchemaVersion: oci.SchemaVersion,
		Manifests:     []v2alpha1.OCIManifest{{MediaType: oci.MediaType, Digest: "sha256:" + digest, Size: size}},
	}

	idxData, err := json.Marshal(idx)
	if err != nil {
		return err
	}

	// Write the JSON string to a file
	err = os.WriteFile(path.Join(dir, "index.json"), idxData, 0644)
	if err != nil {
		return err
	}

	return nil
}

func (o Manifest) GetDigest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string) (string, error) {
	setInternalLog(o.Log)

	if err := mirror.ReexecIfNecessaryForImages([]string{imgRef}...); err != nil {
		return "", err
	}

	srcRef, err := alltransports.ParseImageName(imgRef)
	if err != nil {
		return "", fmt.Errorf("invalid source name %s: %v", imgRef, err)
	}

	img, err := srcRef.NewImageSource(ctx, sourceCtx)
	if err != nil {
		return "", err
	}

	manifestBytes, _, err := img.GetManifest(ctx, nil)
	if err != nil {
		return "", err
	}

	digest, err := manifest.Digest(manifestBytes)
	if err != nil {
		return "", err
	}

	var digestString string
	if strings.Contains(digest.String(), ":") {
		digestString = strings.Split(digest.String(), ":")[1]
	}

	return digestString, nil
}

func setInternalLog(log clog.PluggableLoggerInterface) {
	if internalLog == nil {
		internalLog = log
	}
}
