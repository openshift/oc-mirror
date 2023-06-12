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

	// Take the reference and convert it into a docker image reference.
	reg, ns, name, tag, id := v1alpha2.ParseImageReference(ref)
	dst := libgoref.DockerImageReference{
		Registry:  reg,
		Namespace: ns,
		Name:      name,
		Tag:       tag,
		ID:        id,
	}

	// Because this docker reference is based on a path to the OCI layout
	// we need to convert the name and namespace to lower case to comply with the
	// docker reference spec (see https://github.com/distribution/distribution/blob/main/reference/reference.go).
	// Failure to do this will result in parsing errors for some docker reference parsers that
	// perform strict validation.
	// Example: when "ref" is oci:///Users/bob/temp/ocmirror the uppercase U will cause parsing errors.
	dst.Name = strings.ToLower(dst.Name)
	dst.Namespace = strings.ToLower(dst.Namespace)

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
the ref argument and return the **first available digest** within the layout.

- If this layout stores a multi arch image, it returns the SHA of the manifest list itself.
This handles the case where index.json directly references a multi arch image
as well when a manifest list is indirectly referenced in the blobs directory.

- If this layout stores a single arch image, it returns the SHA of the image manifest.

This function will error when:

- the index.json has no manifests

- the index.json has more than one manifest (assuming that the index is not directly
referencing a multi arch image as described above)

- other unexpected errors encountered during processing
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

	// if manifest has more than one entry, throw error
	if len(idxManifest.Manifests) > 1 {
		return nil, fmt.Errorf("more than one image reference found in OCI layout %s, which usually indicates multiple images are being stored in the layout", ref)
	}

	// grab the first manifest and return its digest
	// using range here despite the prior length check for safety
	// because we could have zero or one entries
	for _, descriptor := range idxManifest.Manifests {
		if descriptor.MediaType.IsImage() {
			// if its an image, make sure it can be obtained
			img, err := ii.Image(descriptor.Digest)
			if err != nil {
				// this is unlikely to happen but if it does, move on to the next item
				continue
			}
			_, err = img.ConfigFile()
			if err != nil {
				// this is unlikely to happen but if it does, move on to the next item
				continue
			}
			return &descriptor.Digest, nil
		} else if descriptor.MediaType.IsIndex() {
			// if its an image index, make sure it can be obtained
			_, err := ii.ImageIndex(descriptor.Digest)
			if err != nil {
				// this is unlikely to happen but if it does, move on to the next item
				continue
			}
			// return the digest for the manifest list
			return &descriptor.Digest, nil
		}
	}
	return nil, fmt.Errorf("OCI layout %s did not contain any usable manifest entries", ref)
}
