package image

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"

	ctrsimgmanifest "github.com/containers/image/v5/manifest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

type ErrInvalidComponent struct {
	image string
	tag   string
}

func (e *ErrInvalidComponent) Error() string {
	return fmt.Sprintf("image %q has invalid component %q", e.image, e.tag)
}

// Associations is a map for Association
// searching
type Associations map[string]Association

// AssociationSet is a set of image Associations
// mapped to their images
type AssociationSet map[string]Associations

// Association between an image and its children, either image layers or child manifests.
type Association struct {
	// Name of the image.
	Name string `json:"name"`
	// Path to image data within archive.
	Path string `json:"path"`
	// ID of the image. Joining this value with "manifests" and Path
	// will produce a path to the image's manifest.
	ID string `json:"id"`
	// TagSymlink of the blob specified by ID.
	// This value must be a filename on disk in the "blobs" dir
	TagSymlink string `json:"tagSymlink"`
	// Type of the image in the context of this tool.
	// See the ImageType enum for options.
	Type ImageType `json:"type"`
	// ManifestDigests of images if the image is a docker manifest list or OCI index.
	// These manifests refer to image manifests by content SHA256 digest.
	// LayerDigests and Manifests are mutually exclusive.
	ManifestDigests []string `json:"manifestDigests,omitempty"`
	// LayerDigests of a single manifest if the image is not a docker manifest list
	// or OCI index. These digests refer to image layer blobs by content SHA256 digest.
	// LayerDigests and Manifests are mutually exclusive.
	LayerDigests []string `json:"layerDigests,omitempty"`
}

// Search will return all Associations for the specificed key
func (as AssociationSet) Search(key string) (values []Association, found bool) {
	assocs, found := as[key]
	values = make([]Association, len(assocs))
	count := 0
	for _, value := range assocs {
		values[count] = value
		count++
	}
	return
}

// UpdateKey will move values under oldKey to newKey in Assocations.
// Old entries will be deleted.
func (as AssociationSet) UpdateKey(oldKey, newKey string) error {

	// make sure we don't delete the
	// same key we just set
	if newKey == oldKey {
		return nil
	}
	values, found := as.Search(oldKey)
	if !found {
		return errors.New("key does not exist in map")
	}
	for _, value := range values {
		as.Add(newKey, value)
	}
	delete(as, oldKey)

	return nil
}

// UpdateValue will update the Association values for a given key
func (as AssociationSet) UpdateValue(key string, value Association) error {

	assocs, found := as[key]
	if !found {
		return errors.New("key does not exist in map")
	}
	assocs[value.Name] = value

	return nil
}

// Add stores a key-value pair in this multimap.
func (as AssociationSet) Add(key string, value Association) {
	assocs, found := as[key]
	if found {
		assocs[value.Name] = value
	} else {
		assocs = make(Associations)
		assocs[value.Name] = value
		as[key] = assocs
	}
}

// Keys returns all unique keys contained in map
func (as AssociationSet) Keys() []string {
	keys := make([]string, len(as))
	count := 0
	for key := range as {
		keys[count] = key
		count++
	}
	return keys
}

// SetContainsKey checks if the AssociationSet map contains a key
func (as AssociationSet) SetContainsKey(key string) (found bool) {
	_, found = as[key]
	return
}

// ContainsKey checks if the Associations map contains the specified key
func (as AssociationSet) ContainsKey(setKey, key string) (found bool) {
	asSet, found := as[setKey]
	if !found {
		return false
	}
	_, found = asSet[key]
	return
}

// Merge Associations into the receiver.
func (as AssociationSet) Merge(in AssociationSet) {
	for _, imageName := range in.Keys() {
		values, _ := in.Search(imageName)
		for _, value := range values {
			as.Add(imageName, value)
		}
	}
}

// Encode Associations in an efficient, opaque format.
func (as AssociationSet) Encode(w io.Writer) error {
	if err := as.validate(); err != nil {
		return fmt.Errorf("invalid image associations: %v", err)
	}
	enc := gob.NewEncoder(w)
	if err := enc.Encode(as); err != nil {
		return fmt.Errorf("error encoding image associations: %v", err)
	}
	return nil
}

// Decode Associations from an opaque format. Only useable if Associations
// was encoded with Encode().
func (as *AssociationSet) Decode(r io.Reader) error {
	dec := gob.NewDecoder(r)
	if err := dec.Decode(as); err != nil {
		return fmt.Errorf("error decoding image associations: %v", err)
	}
	// Update paths for local usage.
	for _, imageName := range as.Keys() {
		assocs, _ := as.Search(imageName)
		for _, assoc := range assocs {
			assoc.Path = filepath.FromSlash(assoc.Path)
			if err := as.UpdateValue(imageName, assoc); err != nil {
				return err
			}
		}
	}
	return nil
}

func (as AssociationSet) validate() error {
	var errs []error
	for _, imageName := range as.Keys() {
		assocs, found := as.Search(imageName)
		if !found {
			return fmt.Errorf("image %q does not exist in association set", imageName)
		}
		for _, a := range assocs {

			if s, ok := imageTypeStrings[a.Type]; ok && s != "" {
				continue
			}
			switch a.Type {
			case TypeInvalid:
				// TypeInvalid is the default value for the concrete type, which means the field was not set.
				errs = append(errs, fmt.Errorf("image %q: must set image type", a.Name))
			default:
				errs = append(errs, fmt.Errorf("image %q: unknown image type %v", a.Name, a.Type))
			}

			if len(a.ManifestDigests) != 0 && len(a.LayerDigests) != 0 {
				errs = append(errs, fmt.Errorf("image %q: child descriptors cannot contain both manifests and image layers", a.Name))
			}
			if len(a.ManifestDigests) == 0 && len(a.LayerDigests) == 0 {
				errs = append(errs, fmt.Errorf("image %q: child descriptors must contain at least one manifest or image layer", a.Name))
			}

			if a.ID == "" {
				errs = append(errs, fmt.Errorf("image %q: tag or ID must be set", a.Name))
			}
		}
	}
	return utilerrors.NewAggregate(errs)
}

func AssociateImageLayers(rootDir string, imgMappings TypedImageMapping) (AssociationSet, utilerrors.Aggregate) {
	errs := []error{}
	bundleAssociations := AssociationSet{}

	skipParse := func(ref string) bool {
		seen := bundleAssociations.SetContainsKey(ref)
		return seen
	}

	localRoot := filepath.Join(rootDir, "v2")
	for image, diskLoc := range imgMappings {
		if diskLoc.Type != imagesource.DestinationFile {
			errs = append(errs, fmt.Errorf("image destination for %q is not type file", image.Ref.String()))
		}
		dirRef := diskLoc.Ref.AsRepository().String()
		imagePath := filepath.Join(localRoot, dirRef)

		// Verify that the dirRef exists before proceeding
		if _, err := os.Stat(imagePath); err != nil {
			errs = append(errs, fmt.Errorf("image %q mapping %q: %v", image, dirRef, err))
			continue
		}

		var tagOrID string
		if image.Ref.Tag != "" {
			tagOrID = image.Ref.Tag
		} else {
			tagOrID = image.Ref.ID
		}

		// TODO(estroz): parallelize
		associations, err := associateImageLayers(image.Ref.String(), localRoot, dirRef, tagOrID, "oc-mirror", image.Category, skipParse)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, association := range associations {
			bundleAssociations.Add(image.Ref.String(), association)
		}
	}

	return bundleAssociations, utilerrors.NewAggregate(errs)
}

func associateImageLayers(image, localRoot, dirRef, tagOrID, defaultTag string, typ ImageType, skipParse func(string) bool) (associations []Association, err error) {
	if skipParse(image) {
		return nil, nil
	}

	manifestPath := filepath.Join(localRoot, filepath.FromSlash(dirRef), "manifests", tagOrID)
	// TODO(estroz): this file mode checking block is likely only necessary
	// for the first recursion leaf since image manifest layers always contain id's,
	// so unroll this component into AssociateImageLayers.

	info, err := os.Lstat(manifestPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, &ErrInvalidComponent{image, tagOrID}
	} else if err != nil {
		return nil, err
	}
	// Tags are always symlinks due to how `oc` libraries mirror manifest files.
	id, tag := tagOrID, tagOrID
	switch m := info.Mode(); {
	case m&fs.ModeSymlink != 0:
		// Tag is the file name, so follow the symlink to the layer ID-named file.
		dst, err := os.Readlink(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("error evaluating image tag symlink: %v", err)
		}
		id = filepath.Base(dst)
	case m.IsRegular():
		// Layer ID is the file name, and no tag exists.
		tag = defaultTag
		if defaultTag != "" {
			// If set, add a subset of the digest to randomize the
			// tag in the event multiple digests are pulled for the same
			// image
			tag = defaultTag + id[7:13]
			manifestDir := filepath.Dir(manifestPath)
			symlink := filepath.Join(manifestDir, tag)
			if err := os.Symlink(info.Name(), symlink); err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("expected symlink or regular file mode, got: %b", m)
	}
	manifestBytes, err := ioutil.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("error reading image manifest file: %v", err)
	}

	association := Association{
		Name:       image,
		Path:       dirRef,
		ID:         id,
		TagSymlink: tag,
		Type:       typ,
	}
	switch mt := ctrsimgmanifest.GuessMIMEType(manifestBytes); mt {
	case "":
		return nil, errors.New("unparseable manifest mediaType")
	case imgspecv1.MediaTypeImageIndex, ctrsimgmanifest.DockerV2ListMediaType:
		list, err := ctrsimgmanifest.ListFromBlob(manifestBytes, mt)
		if err != nil {
			return nil, err
		}
		for _, instance := range list.Instances() {
			digestStr := instance.String()
			// Add manifest references so publish can recursively look up image layers
			// for the manifests of this list.
			association.ManifestDigests = append(association.ManifestDigests, digestStr)
			// Recurse on child manifests, which should be in the same directory
			// with the same file name as it's digest.
			childAssocs, err := associateImageLayers(digestStr, localRoot, dirRef, digestStr, "", typ, skipParse)
			if err != nil {
				return nil, err
			}
			associations = append(associations, childAssocs...)
		}
	default:
		// Treat all others as image manifests.
		manifest, err := ctrsimgmanifest.FromBlob(manifestBytes, mt)
		if err != nil {
			return nil, err
		}
		for _, layerInfo := range manifest.LayerInfos() {
			association.LayerDigests = append(association.LayerDigests, layerInfo.Digest.String())
		}
		// The config is just another blob, so associate it opaquely.
		association.LayerDigests = append(association.LayerDigests, manifest.ConfigInfo().Digest.String())
	}

	associations = append(associations, association)

	return associations, nil
}
