package image

import (
	"bufio"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	ctrsimgmanifest "github.com/containers/image/v5/manifest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

type ErrNoMapping struct {
	image string
}

func (e *ErrNoMapping) Error() string {
	return fmt.Sprintf("image %q has no mirror mapping", e.image)
}

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

type ImageType int

const (
	TypeInvalid ImageType = iota
	TypeOCPRelease
	TypeOperatorCatalog
	TypeOperatorBundle
	TypeOperatorRelatedImage
	TypeGeneric
)

var imageTypeStrings = map[ImageType]string{
	TypeOCPRelease:           "ocpRelease",
	TypeOperatorCatalog:      "operatorCatalog",
	TypeOperatorBundle:       "operatorBundle",
	TypeOperatorRelatedImage: "operatorRelatedImage",
	TypeGeneric:              "generic",
}

func (it ImageType) String() string {
	return imageTypeStrings[it]
}

// Search will return all Associations for the specificed key
func (as AssociationSet) Search(key string) (values []Association, found bool) {
	asSet, found := as[key]
	values = make([]Association, len(asSet))
	count := 0
	for _, value := range asSet {
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

	set, found := as[key]
	if !found {
		return errors.New("key does not exist in map")
	}
	set[value.Name] = value

	return nil
}

// Add stores a key-value pair in this multimap.
func (as AssociationSet) Add(key string, value Association) {
	set, found := as[key]
	if found {
		set[value.Name] = value
	} else {
		set = make(Associations)
		set[value.Name] = value
		as[key] = set
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

// ContainsKey checks if the map contain the specified key
func (as AssociationSet) ContainsKey(key string) (found bool) {
	_, found = as[key]
	return
}

// ContainsKey checks if the map contain the specified key
func (as AssociationSet) SetContainsKey(key, setKey string) (found bool) {
	asSet, found := as[key]
	if !found {
		return false
	}
	_, found = asSet[setKey]
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
			as.UpdateValue(imageName, assoc)
		}
	}
	return nil
}

func (as AssociationSet) validate() error {
	var errs []error
	for _, imageName := range as.Keys() {
		assocs, found := as.Search(imageName)
		if !found {
			return fmt.Errorf("image %q does not exist in assoication set", imageName)
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

// ReadImageMapping reads a mapping.txt file and parses each line into a map k/v.
func ReadImageMapping(mappingsPath string) (map[string]string, error) {
	f, err := os.Open(mappingsPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	mappings := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		text := scanner.Text()
		split := strings.Split(text, "=")
		if len(split) != 2 {
			return nil, fmt.Errorf("mapping %q expected to have exactly one \"=\"", text)
		}
		mappings[strings.TrimSpace(split[0])] = strings.TrimSpace(split[1])
	}

	return mappings, scanner.Err()
}

func AssociateImageLayers(rootDir string, imgMappings map[string]string, images []string, typ ImageType) (AssociationSet, utilerrors.Aggregate) {
	errs := []error{}
	bundleAssociations := AssociationSet{}

	skipParse := func(ref string) bool {
		seen := bundleAssociations.ContainsKey(ref)
		return seen
	}

	localRoot := filepath.Join(rootDir, "v2")
	for _, image := range images {
		dirRef, hasMapping := imgMappings[image]
		if !hasMapping {
			errs = append(errs, &ErrNoMapping{image})
			continue
		}

		// TODO(estroz): maybe just use imgsource.ParseReference() here.
		re := regexp.MustCompile(`.*file://`)

		prefix := re.FindString(dirRef)
		dirRef = strings.TrimPrefix(dirRef, prefix)

		tagIdx := strings.LastIndex(dirRef, ":")
		if tagIdx == -1 {
			errs = append(errs, fmt.Errorf("image %q mapping %q has no tag or digest component", image, dirRef))
		}
		idx := tagIdx
		if idIdx := strings.LastIndex(dirRef, "@"); idIdx != -1 {
			idx = idIdx
		}
		tagOrID := dirRef[idx+1:]

		// afflom - Origin mappings cutoff the arch part of the filename.
		// this adds it back in as needed.
		// This regex finds okd images
		re2 := regexp.MustCompile(`\d\.\d\.\d\-\d\.okd`)
		// if okd
		if re2.MatchString(tagOrID) {
			// This regex finds the release image
			rel := regexp.MustCompile(`(\d){6}$`)
			// This regex finds the file prefix
			re3 := regexp.MustCompile(`(.*)(\d){6}-`)
			// if release image
			if rel.MatchString(tagOrID) {
				// add x86_64 suffix to tagOrID
				tagOrID = fmt.Sprintf("%s%s", tagOrID, "-x86_64")

			} else {
				// All other okd files
				// Get prefix
				tagpre := re3.FindString(tagOrID)
				// Get suffix
				tagsuf := strings.TrimPrefix(tagOrID, tagpre)
				// insert arch in the middle
				tagOrID = fmt.Sprintf("%s%s%s", tagpre, "x86_64-", tagsuf)
			}
		}
		dirRef = dirRef[:idx]
		//logrus.Infof("tagOrID: %s \n dirRef: %s", tagOrID, dirRef)

		imagePath := filepath.Join(localRoot, dirRef)

		// Verify that the dirRef exists before proceeding
		if _, err := os.Stat(imagePath); err != nil {
			errs = append(errs, fmt.Errorf("image %q mapping %q: %v", image, dirRef, err))
			continue
		}

		// TODO(estroz): parallelize
		associations, err := associateImageLayers(image, localRoot, dirRef, tagOrID, typ, skipParse)
		if err != nil {
			errs = append(errs, err)
		}
		for _, association := range associations {
			bundleAssociations.Add(image, association)
		}
	}

	return bundleAssociations, utilerrors.NewAggregate(errs)
}

func associateImageLayers(image, localRoot, dirRef, tagOrID string, typ ImageType, skipParse func(string) bool) (associations []Association, err error) {
	if skipParse(image) {
		return nil, nil
	}

	manifestPath := filepath.Join(localRoot, filepath.FromSlash(dirRef), "manifests", tagOrID)
	// TODO(estroz): this file mode checking block is likely only necessary
	// for the first recursion leaf since image manifest layers always contain id's,
	// so unroll this component into AssociateImageLayers.

	// FIXME(jpower): some of the mappings are passing a tag that are
	// not actually a symlinks on disk. Need to investigate why we
	// receiving invalid mapping and their meaning.
	info, err := os.Lstat(manifestPath)
	if errors.As(err, &os.ErrNotExist) {
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
		tag = ""
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
			childAssocs, err := associateImageLayers(digestStr, localRoot, dirRef, digestStr, typ, skipParse)
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
