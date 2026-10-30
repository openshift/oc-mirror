package image

import (
	"fmt"
	"strings"

	"github.com/opencontainers/go-digest"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
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
	errMessageImage = "%s unable to parse image correctly"
)

// parseImageSpec parses an image string reference an ImageSpec
func parseImageSpec(imgRef string) (ImageSpec, error) {
	transport, reference, name, referenceWithTransport := parseTransport(imgRef)
	digestStr, algorithm, nameAfterDigest, err := parseDigest(imgRef, name)
	if err != nil {
		return ImageSpec{}, err
	}

	tag, nameAfterTag := parseTag(nameAfterDigest, transport)
	if nameAfterTag == "" {
		return ImageSpec{}, fmt.Errorf("unknown image : reference name is empty")
	}

	domain, pathComponent, err := parseDomainAndPath(imgRef, nameAfterTag, transport)
	if err != nil {
		return ImageSpec{}, err
	}

	return ImageSpec{
		Transport:              transport,
		Reference:              reference,
		Name:                   nameAfterTag,
		ReferenceWithTransport: referenceWithTransport,
		Domain:                 domain,
		PathComponent:          pathComponent,
		Tag:                    tag,
		Algorithm:              algorithm,
		Digest:                 digestStr,
	}, nil
}

func parseTransport(imgRef string) (transport, reference, name, referenceWithTransport string) {
	if strings.Contains(imgRef, "://") {
		referenceWithTransport = imgRef
		imgSplit := strings.Split(imgRef, "://")
		if len(imgSplit) == 2 {
			transport = imgSplit[0] + "://"
			reference = imgSplit[1]
			name = imgSplit[1]
		}
	} else {
		transport = consts.DockerProtocol
		reference = imgRef
		name = imgRef
		referenceWithTransport = transport + imgRef
	}
	return transport, reference, name, referenceWithTransport
}

func parseDigest(imgRef, name string) (digestStr, algorithm, nameAfterDigest string, err error) {
	nameAfterDigest = name
	if strings.Contains(name, "@") {
		imgSplit := strings.Split(name, "@")
		if len(imgSplit) > 1 {
			validDigest, parseErr := digest.Parse(imgSplit[1])
			if parseErr != nil {
				return "", "", "", fmt.Errorf("%s unable to parse image correctly : invalid digest", imgRef)
			}
			digestStr = validDigest.Encoded()
			algorithm = validDigest.Algorithm().String()
			nameAfterDigest = imgSplit[0]
		}
	}
	return digestStr, algorithm, nameAfterDigest, nil
}

func parseTag(name, transport string) (tag, nameAfterTag string) {
	nameAfterTag = name
	if strings.Contains(name, ":") {
		if transport == consts.DockerProtocol {
			lastColonIndex := strings.LastIndex(name, ":")
			indexOfDomainPathSeparation := strings.Index(name, "/")
			if indexOfDomainPathSeparation < 0 || (indexOfDomainPathSeparation > 0 && lastColonIndex > indexOfDomainPathSeparation) {
				tag = name[lastColonIndex+1:]
				nameAfterTag = name[:lastColonIndex]
			}
		}
	}
	return tag, nameAfterTag
}

func parseDomainAndPath(imgRef, name, transport string) (domain, pathComponent string, err error) {
	if transport == consts.DockerProtocol {
		imageNameComponents := strings.Split(name, "/")
		switch {
		case len(imageNameComponents) >= 2:
			pathComponent = strings.Join(imageNameComponents[1:], "/")
			domain = imageNameComponents[0]
		case len(imageNameComponents) == 1:
			pathComponent = imageNameComponents[0]
		default:
			return "", "", fmt.Errorf(errMessageImage, imgRef)
		}
	} else {
		pathComponent = name
	}
	return domain, pathComponent, nil
}

// ParseRef accepts transport-prefixed references. Docker references must
// include a tag or digest.
func ParseRef(imgRef string) (ImageSpec, error) {
	imgSpec, err := parseImageSpec(imgRef)
	if err != nil {
		return ImageSpec{}, err
	}
	if imgSpec.Transport == consts.DockerProtocol && imgSpec.Tag == "" && imgSpec.Digest == "" {
		return ImageSpec{}, fmt.Errorf(errMessageImage+" : tag and digest are empty", imgRef)
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

// WithDigest constructs an image reference with a SHA256 digest.
func WithDigest(name, digest string) string {
	return fmt.Sprintf("%s@sha256:%s", name, digest)
}
