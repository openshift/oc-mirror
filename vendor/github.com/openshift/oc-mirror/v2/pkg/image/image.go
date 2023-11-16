package image

import (
	"fmt"
	"strings"
)

func IsImageByDigest(imgRef string) bool {
	return strings.Contains(imgRef, "@")
}

// RefWithoutTransport returns the image reference without the transport prefix.
// `docker://`, `oci://`, `file://` are all examples of transport prefixes.
func RefWithoutTransport(imgRef string) string {
	if strings.Contains(imgRef, "://") {
		transportAndRef := strings.Split(imgRef, "://")
		imgRef = transportAndRef[1]
	}
	return imgRef
}

// PathWithoutDNS trims out the domain name of the image reference.
// It expects the image reference not to have the transport prefix.
// Otherwise, it will return an error.
func PathWithoutDNS(imgRef string) (string, error) {
	if strings.Contains(imgRef, "://") {
		return "", fmt.Errorf("image reference should not contain transport prefix")
	}
	imageName := strings.Split(imgRef, "/")
	if len(imageName) > 2 {
		return strings.Join(imageName[1:], "/"), nil
	} else if len(imageName) == 1 {
		return imageName[0], nil
	} else {
		return "", fmt.Errorf("unable to parse image %s correctly", imgRef)
	}
}

// Hash returns the digest of the image reference.
// TODO this might need to be modified for OCI images.
func Hash(imgRef string) string {
	var hash string
	imgSplit := strings.Split(imgRef, "@")
	if len(imgSplit) > 1 {
		hash = strings.Split(imgSplit[1], ":")[1]
	}
	return hash
}

// PathWithoutDigest trims out the digest or the tag of the image reference.
// It expects the image reference not to have the transport prefix.
func PathWithoutDigestNorTag(imageNameNoDomain string) string {
	return PathWithoutDigest(PathWithoutTag(imageNameNoDomain))
}

// PathWithoutTag trims out the tag of the image reference.
// It expects the image reference not to have the transport prefix.
func PathWithoutTag(imageNameNoDomain string) string {

	if strings.Contains(imageNameNoDomain, ":") {
		nameAndTag := strings.Split(imageNameNoDomain, ":")
		imageNameNoDomain = nameAndTag[0]
	}
	return imageNameNoDomain
}

// PathWithoutDigest trims out the digest of the image reference.
// It expects the image reference not to have the transport prefix.
func PathWithoutDigest(imageNameNoDomain string) string {

	if strings.Contains(imageNameNoDomain, "@") {
		nameAndDigest := strings.Split(imageNameNoDomain, "@")
		imageNameNoDomain = nameAndDigest[0]
	}

	return imageNameNoDomain
}
