package image

import (
	"fmt"
	"strings"
)

func IsImageByDigest(imgRef string) bool {
	return strings.Contains(imgRef, "@")
}

func PathWithoutDNS(imgRef string) (string, error) {

	var imageName []string
	if IsImageByDigest(imgRef) {
		imageNameSplit := strings.Split(imgRef, "@")
		imageName = strings.Split(imageNameSplit[0], "/")
	} else {
		imageName = strings.Split(imgRef, "/")
	}

	if len(imageName) > 2 {
		return strings.Join(imageName[1:], "/"), nil
	} else if len(imageName) == 1 {
		return imageName[0], nil
	} else {
		return "", fmt.Errorf("unable to parse image %s correctly", imgRef)
	}
}

func Hash(imgRef string) string {
	var hash string
	imgSplit := strings.Split(imgRef, "@")
	if len(imgSplit) > 1 {
		hash = strings.Split(imgSplit[1], ":")[1]
	}

	return hash
}
