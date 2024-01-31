package image

import (
	"fmt"
	"strings"

	digest "github.com/opencontainers/go-digest"
)

// specification is sourced from github.com/containers/image/blob/main/docker/reference/reference.go
// Grammar
//
//	reference                       := name [ ":" tag ] [ "@" digest ]
//	name                            := [domain '/'] path-component ['/' path-component]*
//	domain                          := domain-component ['.' domain-component]* [':' port-number]
//	domain-component                := /([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])/
//	port-number                     := /[0-9]+/
//	path-component                  := alphanumeric [separator alphanumeric]*
//	alphanumeric                   := /[a-z0-9]+/
//	separator                       := /[_.]|__|[-]*/
//
//	tag                             := /[\w][\w.-]{0,127}/
//
//	digest                          := digest-algorithm ":" digest-hex
//	digest-algorithm                := digest-algorithm-component [ digest-algorithm-separator digest-algorithm-component ]*
//	digest-algorithm-separator      := /[+.-_]/
//	digest-algorithm-component      := /[A-Za-z][A-Za-z0-9]*/
//	digest-hex                      := /[0-9a-fA-F]{32,}/ ; At least 128 bit digest value
//
//	identifier                      := /[a-f0-9]{64}/
//	short-identifier                := /[a-f0-9]{6,64}/
type ImageSpec struct {
	Transport              string
	Reference              string
	ReferenceWithTransport string
	Name                   string
	Domain                 string
	PathComponent          string
	Tag                    string
	Algorithm              string
	Digest                 string
}

const (
	dockerProtocol  = "docker://"
	errMessageImage = "unable to parse image %s correctly"
)

// It expects the image reference not to have the transport prefix.
// Otherwise, it will return an error.
func ParseRef(imgRef string) (ImageSpec, error) {
	var imgSpec ImageSpec
	if strings.Contains(imgRef, "://") {
		imgSpec.ReferenceWithTransport = imgRef
		imgSplit := strings.Split(imgRef, "://")
		if len(imgSplit) == 2 {
			imgSpec.Transport = imgSplit[0] + "://"
			imgSpec.Reference = imgSplit[1]
			imgSpec.Name = imgSplit[1]
		}
	} else {
		imgSpec.Transport = dockerProtocol
		imgSpec.Reference = imgRef
		imgSpec.Name = imgRef
		imgSpec.ReferenceWithTransport = imgSpec.Transport + imgRef
	}
	if strings.Contains(imgSpec.Name, "@") {
		imgSplit := strings.Split(imgSpec.Name, "@")
		if len(imgSplit) > 1 {
			validDigest, err := digest.Parse(imgSplit[1])
			if err != nil {
				return ImageSpec{}, fmt.Errorf("unable to parse image %s correctly, invalid digest: %v", imgRef, err)
			}
			imgSpec.Digest = validDigest.Encoded()
			imgSpec.Algorithm = validDigest.Algorithm().String()
			imgSpec.Name = imgSplit[0]
		}
	} else if strings.Contains(imgSpec.Name, ":") {
		lastColonIndex := strings.LastIndex(imgSpec.Name, ":")
		imgSpec.Tag = imgSpec.Name[lastColonIndex+1:]
		imgSpec.Name = imgSpec.Name[:lastColonIndex]
	}

	if imgSpec.Name == "" {
		return ImageSpec{}, fmt.Errorf(errMessageImage, imgRef)
	}
	if imgSpec.Transport == dockerProtocol && imgSpec.Tag == "" && imgSpec.Digest == "" {
		return ImageSpec{}, fmt.Errorf(errMessageImage, imgRef)
	}

	if imgSpec.Transport == dockerProtocol {
		imageNameComponents := strings.Split(imgSpec.Name, "/")
		if len(imageNameComponents) > 2 {
			imgSpec.PathComponent = strings.Join(imageNameComponents[1:], "/")
			imgSpec.Domain = imageNameComponents[0]
		} else if len(imageNameComponents) == 1 {
			imgSpec.PathComponent = imageNameComponents[0]
		} else {
			return ImageSpec{}, fmt.Errorf(errMessageImage, imgRef)
		}
	} else {
		imgSpec.PathComponent = imgSpec.Name
	}

	return imgSpec, nil
}

func (i ImageSpec) IsImageByDigest() bool {
	return i.Digest != ""
}
