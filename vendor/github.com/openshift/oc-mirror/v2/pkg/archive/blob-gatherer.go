package archive

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	digest "github.com/opencontainers/go-digest"
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
	imageSubPath, err := imagePathComponents(imgRef)
	if err != nil {
		return nil, err
	}
	imagePath := filepath.Join(o.localStorage, repositoriesSubFolder, imageSubPath)

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

func imagePathComponents(imgRef string) (string, error) {
	var imageName []string
	if strings.Contains(imgRef, "://") {
		transportAndRef := strings.Split(imgRef, "://")
		imgRef = transportAndRef[1]
	}

	imageName = strings.Split(imgRef, "/")

	if len(imageName) > 2 {
		imgRef = strings.Join(imageName[1:], "/")
	} else if len(imageName) == 1 {
		imgRef = imageName[0]
	} else {
		return "", fmt.Errorf("unable to parse image %s correctly", imgRef)
	}

	if strings.Contains(imgRef, "@") {
		nameAndDigest := strings.Split(imgRef, "@")
		imgRef = nameAndDigest[0]
	}

	if strings.Contains(imgRef, ":") {
		nameAndTag := strings.Split(imgRef, ":")
		imgRef = nameAndTag[0]
	}
	return imgRef, nil
}

func isImageByDigest(imgRef string) bool {
	return strings.Contains(imgRef, "@")
}

// func imageHash(imgRef string) string {
// 	var hash string
// 	imgSplit := strings.Split(imgRef, "@")
// 	if len(imgSplit) > 1 {
// 		hash = strings.Split(imgSplit[1], ":")[1]
// 	}

// 	return hash
// }
