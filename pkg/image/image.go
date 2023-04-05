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
	"github.com/openshift/library-go/pkg/image/reference"
	libgoref "github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"k8s.io/klog/v2"
)

var (
	DestinationOCI imagesource.DestinationType = "oci"
	TagLatest      string                      = "latest"
)

type TypedImageReference struct {
	Type       imagesource.DestinationType
	Ref        reference.DockerImageReference
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

func SetDockerClientDefaults(r libgoref.DockerImageReference) libgoref.DockerImageReference {
	oldTag := r.Tag
	r = r.DockerClientDefaults()
	newTag := r.Tag
	if oldTag != TagLatest && newTag == TagLatest && r.ID != "" {
		// OCPBUGS-2633: we cannot use latest, it will make oc-mirror fail
		// yet a tag is still needed because of the way oc-mirror uses tags
		// as symlinks
		// Take away the `sha256:` from the beginning of the digest, and take
		// the first 8 digits
		newTag = strings.TrimPrefix(r.ID, "sha256:")
		if len(newTag) >= 8 {
			newTag = newTag[:8]
		}
	}
	r.Tag = newTag
	return r
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

	reg, ns, name, tag, id := v1alpha2.ParseImageReference(ref)
	dst := libgoref.DockerImageReference{
		Registry:  reg,
		Namespace: ns,
		Name:      name,
		Tag:       tag,
		ID:        id,
	}

	// TODO if manifest does not exist , just do nothing
	// in case of TargetName and TargetTag replacing the original name ,
	// the returned path will not exist on disk
	manifest, err := getManifest(context.Background(), ref)
	if err == nil {
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
		return nil, fmt.Errorf("unable to unmarshall manifest of image : %w", err)
	}
	return manifest, nil
}
