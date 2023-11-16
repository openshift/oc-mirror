package archive

import (
	"os"
	"path/filepath"

	digest "github.com/opencontainers/go-digest"

	"github.com/openshift/oc-mirror/v2/pkg/image"
)

const (
	repositoriesSubFolder = "docker/registry/v2/repositories"
)

type StoreBlobGatherer struct {
	localStorage string
}

func NewStoreBlobGatherer(localStorageLocation string) BlobsGatherer {
	return StoreBlobGatherer{
		localStorage: localStorageLocation,
	}
}
func (o StoreBlobGatherer) GatherBlobs(imgRef string) (map[string]string, error) {
	blobs := map[string]string{}
	imgSpec, err := image.ParseRef(imgRef)
	if err != nil {
		return nil, err
	}

	imagePath := filepath.Join(o.localStorage, repositoriesSubFolder, imgSpec.PathComponent)

	err = filepath.Walk(imagePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == "link" {
			possibleDigest := filepath.Base(filepath.Dir(path))

			if _, err := digest.Parse("sha256:" + possibleDigest); err == nil {
				blobs[possibleDigest] = ""
			} else {
				// this was for a tag
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return blobs, nil
}
