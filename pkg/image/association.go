package image

import (
	"bufio"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	ctrsimgmanifest "github.com/containers/image/v5/manifest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

type MirrorError struct {
	image string
}

func (e *MirrorError) Error() string {
	return fmt.Sprintf("image %q has no mirror mapping", e.image)
}

// Associations is a set of image Associations.
type Associations map[string]Association

// Association between an image and its children, either image layers or child manifests.
type Association struct {
	// Name of the image.
	Name string `json:"name"`
	// Path to image data within archive.
	Path string `json:"path"`
	// ManifestDigests of images if the image is a docker manifest list or OCI index.
	// These manifests refer to image manifests by content SHA256 digest.
	// LayerDigests and Manifests are mutually exclusive.
	ManifestDigests []string `json:"manifestDigests,omitempty"`
	// LayerDigests of a single manifest if the image is not a docker manifest list
	// or OCI index. These digests refer to image layer blobs by content SHA256 digest.
	// LayerDigests and Manifests are mutually exclusive.
	LayerDigests []string `json:"layerDigests,omitempty"`
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
	return nil
}

func (as Associations) validate() error {
	var errs []error
	for _, a := range as {
		if len(a.ManifestDigests) != 0 && len(a.LayerDigests) != 0 {
			errs = append(errs, fmt.Errorf("image %q: child descriptors cannot contain both manifests and image layers", a.Name))
		}
		if len(a.ManifestDigests) == 0 && len(a.LayerDigests) == 0 {
			errs = append(errs, fmt.Errorf("image %q: child descriptors must contain at least one manifest or image layer", a.Name))
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

func AssociateImageLayers(rootDir string, imgMappings map[string]string, images []string) (Associations, error) {

	bundleAssociations := Associations{}
	skipParse := func(ref string) bool {
		_, seen := bundleAssociations[ref]
		return seen
	}

	for _, image := range images {
		dirRef, hasMapping := imgMappings[image]
		if !hasMapping {
			return nil, &MirrorError{image}
		}
		dirRef = strings.TrimPrefix(dirRef, "file://")
		dirRef = filepath.Join(rootDir, "v2", dirRef)

		tagIdx := strings.LastIndex(dirRef, ":")
		if tagIdx == -1 {
			return nil, fmt.Errorf("image %q mapping %q has no tag component", image, dirRef)
		}
		tag := dirRef[tagIdx+1:]
		dirRef = dirRef[:tagIdx]

		associations, err := associateImageLayers(image, dirRef, tag, skipParse)
		if err != nil {
			return nil, fmt.Errorf("image %q mapping %q: %v", image, dirRef, err)
		}
		for _, association := range associations {
			bundleAssociations[association.Name] = association
		}
	}

	return bundleAssociations, nil
}

func associateImageLayers(image, dirRef, tag string, skipParse func(string) bool) (associations []Association, err error) {
	if skipParse(image) {
		return nil, nil
	}

	manifestPath := filepath.Join(dirRef, "manifests", tag)
	manifestBytes, err := ioutil.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("error reading image manifest file: %v", err)
	}

	association := Association{
		Name: image,
		Path: filepath.FromSlash(dirRef),
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
			childAssocs, err := associateImageLayers(digestStr, dirRef, digestStr, skipParse)
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
	}

	associations = append(associations, association)

	return associations, nil
}
