package fixtures

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"sigs.k8s.io/yaml"
)

const (
	imageSpecFile = "image.yaml"
	imageRootFS   = "rootfs"
)

// ImageSpec is the image.yaml file of an image fixture, which describes the image built from
// the rootfs directory next to it.
type ImageSpec struct {
	// Base is the image to build on, pinned by digest. Without it, the image is built from scratch.
	Base string `json:"base,omitempty"`
	// Config is set on top of the configuration of the base image, like the ENTRYPOINT, CMD,
	// ENV, USER, WORKDIR and LABEL instructions of a Dockerfile.
	Config v1.Config `json:"config,omitempty"`
}

// LoadImageSpec reads the image.yaml file of the image fixture in dir.
func LoadImageSpec(dir string) (ImageSpec, error) {
	data, err := os.ReadFile(filepath.Join(dir, imageSpecFile))
	if err != nil {
		return ImageSpec{}, err
	}
	var spec ImageSpec
	if err := yaml.UnmarshalStrict(data, &spec); err != nil {
		return ImageSpec{}, fmt.Errorf("failed to parse %s: %w", filepath.Join(dir, imageSpecFile), err)
	}
	return spec, nil
}

// Image builds the image fixture in dir for platform, on top of base, the image of spec.Base for
// platform, or from scratch when base is nil:
//
//	FROM <base>
//	COPY rootfs /
//	<the configuration of spec>
func Image(base v1.Image, dir string, spec ImageSpec, platform v1.Platform) (v1.Image, error) {
	var copies []dirCopy
	rootfs := filepath.Join(dir, imageRootFS)
	if _, err := os.Stat(rootfs); err == nil {
		copies = append(copies, dirCopy{src: rootfs})
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return build(base, platform, copies, spec.Config)
}

// dirCopy copies the src directory to dest, a relative path, like the COPY instruction of a
// Dockerfile: see dirLayer.
type dirCopy struct {
	src, dest string
	owner     fileOwner
	dirModes  map[string]int64
}

// build builds an image on top of base, or from scratch for platform when base is nil, like a
// Dockerfile with a COPY instruction per element of copies, then the instructions that set config.
func build(base v1.Image, platform v1.Platform, copies []dirCopy, config v1.Config) (v1.Image, error) {
	created := v1.Time{Time: time.Now().UTC()}
	img := base
	if img == nil {
		img = mutate.ConfigMediaType(mutate.MediaType(empty.Image, types.OCIManifestSchema1), types.OCIConfigJSON)
	}
	mediaType, err := layerMediaType(img)
	if err != nil {
		return nil, err
	}
	for _, c := range copies {
		layer, err := dirLayer(c.src, c.dest, c.owner, c.dirModes)
		if err != nil {
			return nil, err
		}
		img, err = mutate.Append(img, mutate.Addendum{Layer: layer, MediaType: mediaType, History: v1.History{
			Created: created, CreatedBy: "COPY " + filepath.Base(c.src) + " /" + c.dest,
		}})
		if err != nil {
			return nil, err
		}
	}

	configFile, err := img.ConfigFile()
	if err != nil {
		return nil, err
	}
	configFile = configFile.DeepCopy()
	configFile.Created = created
	if base == nil {
		configFile.OS = platform.OS
		configFile.Architecture = platform.Architecture
		configFile.Variant = platform.Variant
	}
	setConfig(&configFile.Config, config)
	return mutate.ConfigFile(img, configFile)
}

// setConfig sets the fields of spec on config, the way the instructions of a Dockerfile do.
func setConfig(config *v1.Config, spec v1.Config) {
	if len(spec.Entrypoint) > 0 {
		config.Entrypoint = spec.Entrypoint
		// Like in a Dockerfile, a new entrypoint resets the command of the base image.
		config.Cmd = nil
	}
	if len(spec.Cmd) > 0 {
		config.Cmd = spec.Cmd
	}
	if spec.User != "" {
		config.User = spec.User
	}
	if spec.WorkingDir != "" {
		config.WorkingDir = spec.WorkingDir
	}
	for _, variable := range spec.Env {
		key, _, _ := strings.Cut(variable, "=")
		config.Env = slices.DeleteFunc(config.Env, func(existing string) bool {
			return strings.HasPrefix(existing, key+"=")
		})
		config.Env = append(config.Env, variable)
	}
	if len(spec.Labels) > 0 {
		labels := maps.Clone(config.Labels)
		if labels == nil {
			labels = map[string]string{}
		}
		maps.Copy(labels, spec.Labels)
		config.Labels = labels
	}
}
