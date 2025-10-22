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
	errMessageImage = "%s unable to parse image correctly"
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
				return ImageSpec{}, fmt.Errorf("%s unable to parse image correctly : invalid digest", imgRef)
			}
			imgSpec.Digest = validDigest.Encoded()
			imgSpec.Algorithm = validDigest.Algorithm().String()
			imgSpec.Name = imgSplit[0]
		}
	}
	if strings.Contains(imgSpec.Name, ":") {
		if imgSpec.Transport == dockerProtocol {
			lastColonIndex := strings.LastIndex(imgSpec.Name, ":")
			indexOfDomainPathSeparation := strings.Index(imgSpec.Name, "/")
			if indexOfDomainPathSeparation < 0 || (indexOfDomainPathSeparation > 0 && lastColonIndex > indexOfDomainPathSeparation) {
				imgSpec.Tag = imgSpec.Name[lastColonIndex+1:]
				imgSpec.Name = imgSpec.Name[:lastColonIndex]
			}
		}

	}

	if imgSpec.Name == "" {
		return ImageSpec{}, fmt.Errorf("unknown image : reference name is empty")
	}
	if imgSpec.Transport == dockerProtocol && imgSpec.Tag == "" && imgSpec.Digest == "" {
		return ImageSpec{}, fmt.Errorf(errMessageImage+" : tag and digest are empty", imgRef)
	}

	if imgSpec.Transport == dockerProtocol {
		imageNameComponents := strings.Split(imgSpec.Name, "/")
		if len(imageNameComponents) >= 2 {
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

func (i ImageSpec) IsImageByDigestOnly() bool {
	return i.Tag == "" && i.Digest != ""
}

// OCPBUGS-33196
// check to see if we have both tag and digest
// this is really bad practice on the part of the image creator
// we need to handle this edge case
func (i ImageSpec) IsImageByTagAndDigest() bool {
	return len(strings.Split(i.Reference, ":")) > 2 && strings.Contains(i.Reference, "@")
}

func WithMaxNestedPaths(imageRef string, maxNestedPaths int) (string, error) {
	if maxNestedPaths == 0 {
		return imageRef, nil
	}
	spec, err := ParseRef(imageRef)
	if err != nil {
		return "", err
	}
	components := strings.Split(spec.PathComponent, "/")
	// initialize the output
	pathWithMaxNexted := spec.PathComponent
	if len(components) > maxNestedPaths {
		// we keep (maxNestedPaths - 1) components the way they are
		pathWithMaxNexted = strings.Join(components[:maxNestedPaths-1], "/")
		// we concatenate the rest with `-` and this becomes the last component
		lastPathComponent := strings.Join(components[maxNestedPaths-1:], "-")
		pathWithMaxNexted = strings.Join([]string{pathWithMaxNexted, lastPathComponent}, "/")

	}
	return strings.Replace(imageRef, spec.PathComponent, pathWithMaxNexted, 1), nil
}

func (i ImageSpec) ComponentName() string {
	if strings.Contains(i.PathComponent, "/") {
		pathComponents := strings.Split(i.PathComponent, "/")
		return pathComponents[len(pathComponents)-1]
	} else {
		return i.PathComponent
	}
}

func (i ImageSpec) SetTag(tag string) ImageSpec {
	oldTag := i.Tag
	i.Tag = tag
	i.Reference = strings.Replace(i.Reference, oldTag, tag, 1)
	i.ReferenceWithTransport = strings.Replace(i.ReferenceWithTransport, oldTag, tag, 1)
	return i
}
