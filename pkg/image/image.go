package image

import (
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	libgoref "github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
)

var (
	DestinationOCI imagesource.DestinationType = "oci"
)

// GetVersionsFromImage gets the set of versions after stripping a dash-suffix,
// effectively stripping out timestamps. Example: tag "v4.11-1648566121" becomes version "v4.11"
func GetVersionsFromImage(catalog string) (map[string]int, error) {
	versionTags, err := GetTagsFromImage(catalog)
	if err != nil {
		return nil, err
	}
	versions := make(map[string]int)
	for _, vt := range versionTags {
		v := strings.Split(vt, "-")
		versions[v[0]] += 1
	}
	return versions, nil
}

// GetTagsFromImage gets the tags for an image
func GetTagsFromImage(image string) ([]string, error) {
	repo, err := name.NewRepository(image)
	if err != nil {
		return nil, err
	}
	tags, err := remote.List(repo, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	return tags, err
}

/*
ParseReference is a wrapper function of imagesource.ParseReference

	It provides support for oci: prefixes
*/
func ParseReference(ref string) (imagesource.TypedImageReference, error) {
	if !strings.HasPrefix(ref, "oci:") {
		return imagesource.ParseReference(ref)
	}

	dstType := DestinationOCI
	ref = strings.TrimPrefix(ref, "oci:")
	ref = strings.TrimPrefix(ref, "//") //it could be that there is none
	ref = strings.TrimPrefix(ref, "/")  // case of full path

	dst, err := libgoref.Parse(ref)
	if err != nil {
		return imagesource.TypedImageReference{Ref: dst, Type: dstType}, fmt.Errorf("%q is not a valid image reference: %v", ref, err)
	}
	return imagesource.TypedImageReference{Ref: dst, Type: dstType}, nil
}

// parseImageName returns the registry, organisation, repository, tag and digest
// from the imageName.
// It can handle both remote and local images.
func ParseImageReference(imageName string) (string, string, string, string, string) {
	registry, org, repo, tag, sha := "", "", "", "", ""
	imageName = TrimProtocol(imageName)
	imageName = strings.TrimPrefix(imageName, "/")
	imageName = strings.TrimSuffix(imageName, "/")
	tmp := strings.Split(imageName, "/")

	registry = tmp[0]
	img := strings.Split(tmp[len(tmp)-1], ":")
	if len(tmp) > 2 {
		org = strings.Join(tmp[1:len(tmp)-1], "/")
	}
	if len(img) > 1 {
		if strings.Contains(img[0], "@") {
			nm := strings.Split(img[0], "@")
			repo = nm[0]
			sha = img[1]
		} else {
			repo = img[0]
			tag = img[1]
		}
	} else {
		repo = img[0]
	}

	return registry, org, repo, tag, sha
}

// trimProtocol removes oci://, file:// or docker:// from
// the parameter imageName
func TrimProtocol(imageName string) string {
	imageName = strings.TrimPrefix(imageName, "oci:")
	imageName = strings.TrimPrefix(imageName, "file:")
	imageName = strings.TrimPrefix(imageName, "docker:")
	imageName = strings.TrimPrefix(imageName, "//")

	return imageName
}

func IsFBCOCI(imageRef string) bool {
	return strings.HasPrefix(imageRef, "oci:")
}
