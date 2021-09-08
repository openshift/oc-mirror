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
	"strings"

	ctrsimgmanifest "github.com/containers/image/v5/manifest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
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

// Associations is a set of image Associations.
type Associations map[string]Association

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

// Merge Associations into the receiver.
func (as Associations) Merge(in Associations) {
	for k, v := range in {
		as[k] = v
	}
}

// Encode Associations in an efficient, opaque format.
func (as Associations) Encode(w io.Writer) error {
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
func (as *Associations) Decode(r io.Reader) error {
	dec := gob.NewDecoder(r)
	if err := dec.Decode(as); err != nil {
		return fmt.Errorf("error decoding image associations: %v", err)
	}
	// Update paths for local usage.
	for k, assoc := range *as {
		assoc.Path = filepath.FromSlash(assoc.Path)
		(*as)[k] = assoc
	}
	return nil
}

func (as Associations) validate() error {
	var errs []error
	for _, a := range as {
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
	return utilerrors.NewAggregate(errs)
}

// ReadImageMapping reads a mapping.txt file and parses each line into a map k/v.
func ReadImageMapping(mappingsPath string) (map[string]string, error) {
	f, err := os.Open(mappingsPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	mappings := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		text := scanner.Text()
		split := strings.Split(text, "=")
		if len(split) != 2 {
			return nil, fmt.Errorf("mapping %q expected to have exactly one \"=\"", text)
		}
		logrus.Debugf("read mapping: %s=%s", split[0], split[1])
		mappings[strings.TrimSpace(split[0])] = strings.TrimSpace(split[1])
	}

	return mappings, scanner.Err()
}

func AssociateImageLayers(rootDir string, imgMappings map[string]string, images []string) (Associations, utilerrors.Aggregate) {
	errs := []error{}
	bundleAssociations := Associations{}
	skipParse := func(ref string) bool {
		_, seen := bundleAssociations[ref]
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
		dirRef = strings.TrimPrefix(dirRef, "file://")

		tagIdx := strings.LastIndex(dirRef, ":")
		if tagIdx == -1 {
			errs = append(errs, fmt.Errorf("image %q mapping %q has no tag or digest component", image, dirRef))
		}
		idx := tagIdx
		if idIdx := strings.LastIndex(dirRef, "@"); idIdx != -1 {
			idx = idIdx
		}
		tagOrID := dirRef[idx+1:]
		dirRef = dirRef[:idx]

		imagePath := filepath.Join(localRoot, dirRef)

		// Verify that the dirRef exists before proceeding
		if _, err := os.Stat(imagePath); err != nil {
			errs = append(errs, fmt.Errorf("image %q mapping %q: %v", image, dirRef, err))
			continue
		}

		// TODO(estroz): parallelize
		associations, err := associateImageLayers(image, localRoot, dirRef, tagOrID, skipParse)
		if err != nil {
			errs = append(errs, err)
		}
		for _, association := range associations {
			bundleAssociations[association.Name] = association
		}
	}

	return bundleAssociations, utilerrors.NewAggregate(errs)
}

func associateImageLayers(image, localRoot, dirRef, tagOrID string, skipParse func(string) bool) (associations []Association, err error) {
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
			childAssocs, err := associateImageLayers(digestStr, localRoot, dirRef, digestStr, skipParse)
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
