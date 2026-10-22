package testimages

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// epoch is the modification time of every tar entry, so layers are reproducible.
var epoch = time.Unix(0, 0).UTC()

var (
	linuxAMD64   = v1.Platform{OS: "linux", Architecture: "amd64"}
	linuxARM64   = v1.Platform{OS: "linux", Architecture: "arm64"}
	linuxS390X   = v1.Platform{OS: "linux", Architecture: "s390x"}
	linuxPPC64LE = v1.Platform{OS: "linux", Architecture: "ppc64le"}
)

// mediaTypes selects the manifest format of a built image.
type mediaTypes int

const (
	// dockerMediaTypes produces Docker schema 2 manifests and manifest lists.
	dockerMediaTypes mediaTypes = iota
	// ociMediaTypes produces OCI manifests and image indexes.
	ociMediaTypes
)

// layerFile is a regular file to put in a layer, at a slash-separated path
// relative to the image root.
type layerFile struct {
	path    string
	content []byte
}

// imageSpec describes a single-platform image.
type imageSpec struct {
	platform   v1.Platform
	mediaTypes mediaTypes
	labels     map[string]string
	// layers holds the files of each layer, from bottom to top.
	layers [][]layerFile
}

// buildImage builds a reproducible image from spec.
func buildImage(spec imageSpec) (v1.Image, error) {
	img := empty.Image
	layerMediaType := types.DockerLayer
	if spec.mediaTypes == ociMediaTypes {
		img = mutate.MediaType(img, types.OCIManifestSchema1)
		img = mutate.ConfigMediaType(img, types.OCIConfigJSON)
		layerMediaType = types.OCILayer
	}

	for _, files := range spec.layers {
		layer, err := buildLayer(files, layerMediaType)
		if err != nil {
			return nil, err
		}
		img, err = mutate.AppendLayers(img, layer)
		if err != nil {
			return nil, fmt.Errorf("appending layer: %w", err)
		}
	}

	cfg, err := img.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	cfg = cfg.DeepCopy()
	cfg.OS = spec.platform.OS
	cfg.Architecture = spec.platform.Architecture
	cfg.Variant = spec.platform.Variant
	cfg.Config.Labels = spec.labels

	img, err = mutate.ConfigFile(img, cfg)
	if err != nil {
		return nil, fmt.Errorf("setting config: %w", err)
	}
	return img, nil
}

// buildMultiArchImage builds an image index with one image per platform, each
// built from spec with its platform replaced.
func buildMultiArchImage(spec imageSpec, platforms []v1.Platform) (v1.ImageIndex, error) {
	var images []v1.Image
	for _, platform := range platforms {
		spec.platform = platform
		img, err := buildImage(spec)
		if err != nil {
			return nil, fmt.Errorf("building %s image: %w", platform.String(), err)
		}
		images = append(images, img)
	}
	return newIndex(spec.mediaTypes, platforms, images)
}

// newIndex returns an image index of images, where images[i] is for platforms[i].
func newIndex(mt mediaTypes, platforms []v1.Platform, images []v1.Image) (v1.ImageIndex, error) {
	if len(platforms) != len(images) {
		return nil, fmt.Errorf("got %d platforms for %d images", len(platforms), len(images))
	}
	idx := mutate.IndexMediaType(empty.Index, types.DockerManifestList)
	if mt == ociMediaTypes {
		idx = mutate.IndexMediaType(empty.Index, types.OCIImageIndex)
	}
	for i, img := range images {
		p := platforms[i]
		idx = mutate.AppendManifests(idx, mutate.IndexAddendum{
			Add:        img,
			Descriptor: v1.Descriptor{Platform: &p},
		})
	}
	return idx, nil
}

// buildLayer builds a reproducible gzipped tar layer containing files and all of
// their parent directories.
func buildLayer(files []layerFile, mediaType types.MediaType) (v1.Layer, error) {
	entries := map[string]*tar.Header{}
	for _, f := range files {
		for dir := path.Dir(f.path); dir != "." && dir != "/"; dir = path.Dir(dir) {
			entries[dir+"/"] = &tar.Header{Typeflag: tar.TypeDir, Name: dir + "/", Mode: 0o755}
		}
		entries[f.path] = &tar.Header{Typeflag: tar.TypeReg, Name: f.path, Mode: 0o644, Size: int64(len(f.content))}
	}
	contents := map[string][]byte{}
	for _, f := range files {
		contents[f.path] = f.content
	}

	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	// Sorting lists every directory before its contents.
	sort.Strings(names)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, n := range names {
		hdr := entries[n]
		hdr.ModTime = epoch
		hdr.Format = tar.FormatPAX
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, fmt.Errorf("writing tar header %s: %w", n, err)
		}
		if hdr.Typeflag == tar.TypeReg {
			if _, err := tw.Write(contents[n]); err != nil {
				return nil, fmt.Errorf("writing tar entry %s: %w", n, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("closing tar: %w", err)
	}

	data := buf.Bytes()
	return tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}, tarball.WithMediaType(mediaType))
}

// filesFromDir reads every regular file under dir/<subdir> for each subdir, and
// returns them with paths relative to dir, prefixed with prefix.
// Each file's content goes through transform, which may be nil.
func filesFromDir(dir, prefix string, subdirs []string, transform func(path string, content []byte) ([]byte, error)) ([]layerFile, error) {
	var files []layerFile
	for _, subdir := range subdirs {
		root := filepath.Join(dir, subdir)
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.Type().IsRegular() {
				return err
			}
			content, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if transform != nil {
				if content, err = transform(p, content); err != nil {
					return err
				}
			}
			rel, err := filepath.Rel(dir, p)
			if err != nil {
				return err
			}
			files = append(files, layerFile{path: path.Join(prefix, filepath.ToSlash(rel)), content: content})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", root, err)
		}
	}
	return files, nil
}
