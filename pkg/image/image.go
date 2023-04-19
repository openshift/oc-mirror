package image

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	gcr "github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	libgoref "github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"k8s.io/klog/v2"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
)

var (
	DestinationOCI imagesource.DestinationType = "oci"
)

type TypedImageReference struct {
	Type       imagesource.DestinationType   // the destination type for this image
	Ref        libgoref.DockerImageReference // docker image reference (NOTE: if OCIFBCPath is not empty, this is just an approximation of a docker reference)
	OCIFBCPath string                        // the path to the OCI layout on the file system. Will be empty string if reference is not OCI.
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
	digest, err := getFirstDigestFromPath(ref)
	if err != nil {
		// invalidate the ID
		dst.ID = ""

		if errors.Is(err, os.ErrNotExist) {
			// we know the error could be due to ref not pointing at a real directory
			klog.V(1).Infof("path to oci layout does not exist (this is expected): %v", err)
		} else {
			// be noisy about this, but don't fail
			klog.Infof("unexpected error encountered while getting digest from oci layout path: %v", err)
		}

	} else {
		dst.ID = digest.String()
	}
	return TypedImageReference{Ref: dst, Type: dstType, OCIFBCPath: ref}, nil
}

/*
getFirstDigestFromPath will inspect a OCI layout path provided by
the ref argument and return the **first** digest discovered. If this is
a multi arch image, it returns the SHA of the manifest list itself.
If this is a single arch image, it returns the config SHA. This function
will error if no manifests were found in the top level index.json. If there
are more than one manifest, the other entries are ignored and a log message
is generated.
*/
func getFirstDigestFromPath(ref string) (*v1.Hash, error) {
	filepath := v1alpha2.TrimProtocol(ref)
	layoutPath, err := gcr.FromPath(filepath)
	if err != nil {
		return nil, err
	}
	ii, err := layoutPath.ImageIndex()
	if err != nil {
		return nil, err
	}

	idxManifest, err := ii.IndexManifest()
	if err != nil {
		return nil, err
	}

	// Handle use case where the index.json does not use
	// indirection to point at a manifest list in the blobs directory.
	// Do this by looking at the media type... its normally not present
	// but when it is, this is a direct reference to the manifest list.
	if idxManifest.MediaType.IsIndex() {
		hash, err := ii.Digest()
		if err != nil {
			return nil, err
		}
		return &hash, nil
	}

	// if manifest has more than one entry, indicate that something is probably not right with this OCI layout, but don't error out
	if len(idxManifest.Manifests) > 1 {
		klog.Infof("more than one image reference found in OCI layout %s using first entry only", ref)
	}

	// grab the first manifest an return its digest
	for _, descriptor := range idxManifest.Manifests {
		if descriptor.MediaType.IsImage() {
			// if its an image, get the config hash and return that value
			img, err := ii.Image(descriptor.Digest)
			if err != nil {
				return nil, err
			}
			hash, err := img.ConfigName()
			if err != nil {
				return nil, err
			}
			return &hash, nil
		} else {
			// return the digest for the manifest list
			return &descriptor.Digest, nil
		}
	}
	return nil, fmt.Errorf("OCI layout %s did not contain any manifest entries", ref)
}
