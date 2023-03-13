package image

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/oci/layout"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	libgoref "github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"k8s.io/klog/v2"
)

var (
	DestinationOCI imagesource.DestinationType = "oci"
)

// type ImageReferenceInterface interface {
// 	String() string
// 	Equal(other ImageReferenceInterface) bool
// 	DockerClientDefaults() ImageReferenceInterface
// 	AsV2() ImageReferenceInterface
// 	Exact() string
// }

type TypedImageReference struct {
	Type       imagesource.DestinationType
	Ref        libgoref.DockerImageReference
	OCIFBCPath string
}

func (t TypedImageReference) String() string {
	switch t.Type {
	case imagesource.DestinationFile:
		return fmt.Sprintf("file://%s", t.Ref.Exact())
	case imagesource.DestinationS3:
		return fmt.Sprintf("s3://%s", t.Ref.Exact())
	case DestinationOCI:
		return fmt.Sprintf("oci://%s", t.Ref.Exact())
	default:
		return t.Ref.Exact()
	}
}

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
func ParseReference(ref string) (TypedImageReference, error) {
	if !strings.HasPrefix(ref, v1alpha2.OCITransportPrefix) {
		orig, err := imagesource.ParseReference(ref)
		if err != nil {
			return TypedImageReference{}, err
		}
		return TypedImageReference{
			Ref:  orig.Ref,
			Type: orig.Type,
		}, nil
	}

	dstType := DestinationOCI

	// TODO: this assumes you can parse it into a docker image ref (and you can’t do that since its just a path).
	reg, ns, name, tag, id := v1alpha2.ParseImageReference(ref)
	dst := libgoref.DockerImageReference{
		Registry:  reg,
		Namespace: ns,
		Name:      name,
		Tag:       tag,
		ID:        id,
	}

	// if manifest does not exist (in case of TargetName and TargetTag replacing the original name,
	// the returned path will not exist on disk), invalidate the ID since parsing the path
	// to the OCI layout won't mean anything, and you'll likely get bogus
	// information for the ID
	manifest, err := getManifest(context.Background(), ref)
	if err != nil {
		// invalidate the ID
		dst.ID = ""
		klog.Infof("%v", err)
	} else {
		dst.ID = string(manifest.ConfigInfo().Digest)
	}
	return TypedImageReference{Ref: dst, Type: dstType, OCIFBCPath: ref}, nil
}

// getManifest reads the manifest of the OCI FBC image
// and returns it as a go structure of type manifest.Manifest
func getManifest(ctx context.Context, imgPath string) (manifest.Manifest, error) {
	imgRef, err := alltransports.ParseImageName(imgPath)
	if err != nil {
		return nil, fmt.Errorf("unable to parse reference %s: %v", imgPath, err)
	}
	imgsrc, err := imgRef.NewImageSource(ctx, nil)
	defer func() {
		if imgsrc != nil {
			err = imgsrc.Close()
			if err != nil {
				klog.V(3).Infof("%s is not closed", imgsrc)
			}
		}
	}()
	if err != nil {
		if err == layout.ErrMoreThanOneImage {
			return nil, errors.New("multiple catalogs in the same location is not supported: https://github.com/openshift/oc-mirror/blob/main/TROUBLESHOOTING.md#error-examples")
		}
		return nil, fmt.Errorf("unable to create ImageSource for %s: %v", err, imgPath)
	}
	manifestBlob, manifestType, err := imgsrc.GetManifest(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to get manifest blob from image : %w", err)
	}
	manifest, err := manifest.FromBlob(manifestBlob, manifestType)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal manifest of image : %w", err)
	}
	return manifest, nil
}
