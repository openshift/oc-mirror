package builder

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/match"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"k8s.io/klog/v2"
)

const (
	// Mode constants from the USTAR spec:
	// See http://pubs.opengroup.org/onlinepubs/9699919799/utilities/pax.html#tag_20_92_13_06
	c_ISUID = 04000 // Set uid
	c_ISGID = 02000 // Set gid
	c_ISVTX = 01000 // Save text (sticky bit)
)

// ImageBuilder use an OCI workspace to add layers and change configuration to images.
type ImageBuilder struct {
	NameOpts   []name.Option
	RemoteOpts []remote.Option
	Logger     klog.Logger
}

// ErrInvalidReference is returned the target reference is a digest.
type ErrInvalidReference struct {
	image string
}

func (e ErrInvalidReference) Error() string {
	return fmt.Sprintf("target reference %q must have a tag reference", e.image)
}

// NewImageBuilder creates a new instance of an ImageBuilder.
func NewImageBuilder(nameOpts []name.Option, remoteOpts []remote.Option) *ImageBuilder {
	b := &ImageBuilder{
		NameOpts:   nameOpts,
		RemoteOpts: remoteOpts,
	}
	b.init()
	return b
}

func (b *ImageBuilder) init() {
	if b.Logger == (logr.Logger{}) {
		b.Logger = klog.NewKlogr()
	}
}

/*
configUpdateFunc allows callers of ImageBuilder.Run to modify the *v1.ConfigFile argument as appropriate for
the circumstances.
*/
type configUpdateFunc func(*v1.ConfigFile)

/*
Run modifies and pushes the catalog image existing in an OCI layout. The image configuration will be updated
with the required labels and any provided layers will be appended.

# Arguments

• ctx: a cancellation context

• targetRef: a docker image reference

• layoutPath: an OCI image layout path

• update: an optional function that allows callers to modify the *v1.ConfigFile if necessary

• layers: zero or more layers to add to the images discovered during processing

# Returns

error: non-nil on error, nil otherwise
*/
func (b *ImageBuilder) Run(ctx context.Context, targetRef string, layoutPath layout.Path, update configUpdateFunc, layers ...v1.Layer) error {
	b.init()
	var v2format bool

	// Target can't have a digest since we are
	// adding layers and possibly updating the
	// configuration. This will result in a failure
	// due to computed hash differences.
	targetIdx := strings.Index(targetRef, "@")
	if targetIdx != -1 {
		return &ErrInvalidReference{targetRef}
	}

	tag, err := name.NewTag(targetRef, b.NameOpts...)
	if err != nil {
		return err
	}

	idx, err := layoutPath.ImageIndex()
	if err != nil {
		return err
	}
	// make a copy of the original manifest for later
	originalIdxManifest, err := idx.IndexManifest()
	if err != nil {
		return err
	}
	originalIdxManifest = originalIdxManifest.DeepCopy()

	// process the image index for updates to images discovered along the way
	resultIdx, err := b.processImageIndex(ctx, idx, &v2format, update, targetRef, layers...)
	if err != nil {
		return err
	}

	// Ensure the index media type is a docker manifest list
	// if child manifests are docker V2 schema
	if v2format {
		resultIdx = mutate.IndexMediaType(resultIdx, types.DockerManifestList)
	}
	// get the hashes from the original manifest since we need to remove them
	originalHashes := []v1.Hash{}
	for _, desc := range originalIdxManifest.Manifests {
		originalHashes = append(originalHashes, desc.Digest)
	}
	// write out the index, replacing the old value
	err = layoutPath.ReplaceIndex(resultIdx, match.Digests(originalHashes...))
	if err != nil {
		return err
	}
	// "Pull" the updated index
	idx, err = layoutPath.ImageIndex()
	if err != nil {
		return err
	}
	// while it's entirely valid to have nested "manifest list" (i.e. an ImageIndex) within an OCI layout,
	// this does NOT work for remote registries. So if we have those, then we need to get the nested
	// ImageIndex and push that to the remote registry. In theory there could be any number of nested
	// ImageIndexes, but in practice, there's only one level deep, and its a "singleton".
	topLevelIndexManifest, err := idx.IndexManifest()
	if err != nil {
		return err
	}
	var imageIndexToPush v1.ImageIndex
	for _, descriptor := range topLevelIndexManifest.Manifests {
		if descriptor.MediaType.IsImage() {
			// if we find an image, then this top level index can be used to push to remote registry
			imageIndexToPush = idx
			// no need to look any further
			break
		} else if descriptor.MediaType.IsIndex() {
			// if we find an image index, we can push that to the remote registry
			imageIndexToPush, err = idx.ImageIndex(descriptor.Digest)
			if err != nil {
				return err
			}
			// we're not going to look any deeper or look for other indexes at this level
			break
		}
	}
	// push to the remote
	return remote.WriteIndex(tag, imageIndexToPush, b.RemoteOpts...)
}

/*
processImageIndex is a recursive helper function that allows for traversal of the hierarchy of
parent/child indexes that can exist for a multi arch image. There's always
at least one index at the root since this is an OCI layout that we're dealing with.
In theory there can be "infinite levels" of "index indirection" for multi arch images, but typically
its only two levels deep (i.e. index.json itself which is level one, and the manifest list
defined in the blobs directory, which is level two).

Each image that is encountered is updated using the update function (if provided) and whatever layers are provided.

# Arguments

• ctx: a cancellation context

• idx: the "current" image index for this stage of recursion

• v2format: a boolean used to keep track of the type of image we're dealing with. false means OCI media types
should be used and true means docker v2s2 media types should be used

• update: an optional function that allows callers to modify the *v1.ConfigFile if necessary

• targetRef: the docker image reference, which is only used for error reporting in this function

• layers: zero or more layers to add to the images discovered during processing

# Returns

• v1.ImageIndex: The resulting image index after processing has completed. Will be nil if an error occurs, otherwise non-nil.

• error: non-nil if an error occurs, nil otherwise
*/
func (b *ImageBuilder) processImageIndex(ctx context.Context, idx v1.ImageIndex, v2format *bool, update configUpdateFunc, targetRef string, layers ...v1.Layer) (v1.ImageIndex, error) {
	var resultIdx v1.ImageIndex
	resultIdx = idx
	idxManifest, err := idx.IndexManifest()
	if err != nil {
		return nil, err
	}
	for _, manifest := range idxManifest.Manifests {
		currentHash := *manifest.Digest.DeepCopy()
		switch manifest.MediaType {
		case types.DockerManifestList, types.OCIImageIndex:
			innerIdx, err := idx.ImageIndex(currentHash)
			if err != nil {
				return nil, err
			}
			// recursive call
			processedIdx, err := b.processImageIndex(ctx, innerIdx, v2format, update, targetRef, layers...)
			if err != nil {
				return nil, err
			}
			resultIdx = processedIdx
			// making an assumption here that at any given point in the parent/child
			// hierarchy, there's only a single image index entry
			return resultIdx, nil
		case types.DockerManifestSchema2:
			*v2format = true
		case types.OCIManifestSchema1:
			*v2format = false
		default:
			return nil, fmt.Errorf("image %q: unsupported manifest format %q", targetRef, manifest.MediaType)
		}

		img, err := idx.Image(currentHash)
		if err != nil {
			return nil, err
		}

		// Add new layers to image.
		// Ensure they have the right media type.
		if len(layers) > 0 {
			var mt types.MediaType
			if *v2format {
				mt = types.DockerLayer
			} else {
				mt = types.OCILayer
			}
			additions := make([]mutate.Addendum, 0, len(layers))
			for _, layer := range layers {
				additions = append(additions, mutate.Addendum{Layer: layer, MediaType: mt})
			}
			img, err = mutate.Append(img, additions...)
			if err != nil {
				return nil, err
			}
		}

		if update != nil {
			// Update image config
			cfg, err := img.ConfigFile()
			if err != nil {
				return nil, err
			}
			update(cfg)
			img, err = mutate.Config(img, cfg.Config)
			if err != nil {
				return nil, err
			}
		}

		desc, err := partial.Descriptor(img)
		if err != nil {
			return nil, err
		}

		// if the platform is not set, we need to attempt to do something about that
		if desc.Platform == nil {
			if manifest.Platform != nil {
				// use the value from the manifest
				desc.Platform = manifest.Platform
			} else {
				if config, err := img.ConfigFile(); err != nil {
					// we can't get the config file so fall back to linux/amd64
					desc.Platform = &v1.Platform{Architecture: "amd64", OS: "linux"}
				} else {
					// if one of the required values is missing, fall back to linux/amd64
					if config.Architecture == "" || config.OS == "" {
						desc.Platform = &v1.Platform{Architecture: "amd64", OS: "linux"}
					} else {
						// use the value provided by the image config
						desc.Platform = &v1.Platform{Architecture: config.Architecture, OS: config.OS}
					}
				}
			}
		}
		add := mutate.IndexAddendum{
			Add:        img,
			Descriptor: *desc,
		}
		modifiedIndex := mutate.AppendManifests(mutate.RemoveManifests(resultIdx, match.Digests(currentHash)), add)
		resultIdx = modifiedIndex
	}
	return resultIdx, nil
}

/*
CreateLayout will create an OCI image layout from an image or return
a layout path from an existing OCI layout.

# Arguments

• srcRef: if empty string, the dir argument is used for the layout.Path, otherwise
this value is used to pull an image into dir.

• dir: a pre-populated OCI layout directory if srcRef is empty string, otherwise
this directory will be created

# Returns

• layout.Path: a OCI layout path if successful or an empty string if an error occurs

• error: non-nil if an error occurs, nil otherwise
*/
func (b *ImageBuilder) CreateLayout(srcRef, dir string) (layout.Path, error) {
	b.init()
	if srcRef == "" {
		b.Logger.V(1).Info("Using existing OCI layout to " + dir)
		return layout.FromPath(dir)
	}

	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return "", err
	}
	// Pull source reference image
	ref, err := name.ParseReference(srcRef, b.NameOpts...)
	if err != nil {
		return "", err
	}
	idx, err := remote.Index(ref, b.RemoteOpts...)
	if err != nil {
		return "", err
	}
	b.Logger.V(1).Info("Writing OCI layout to " + dir)
	return layout.Write(dir, idx)
}

// LayerFromPath will write the contents of the path(s) the target
// directory and build a v1.Layer
func LayerFromPath(targetPath, path string) (v1.Layer, error) {
	return LayerFromPathWithUidGid(targetPath, path, -1, -1)
}

// LayerFromPath will write the contents of the path(s) the target
// directory specifying the target UID/GID and build a v1.Layer.
// Use gid = -1 , uid = -1 if you don't want to override.
func LayerFromPathWithUidGid(targetPath, path string, uid int, gid int) (v1.Layer, error) {
	var b bytes.Buffer
	tw := tar.NewWriter(&b)

	pathInfo, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	processPaths := func(hdr *tar.Header, info os.FileInfo, fp string) error {
		if !info.IsDir() {
			hdr.Size = info.Size()
		}
		hdr.ChangeTime = time.Now()
		if info.Mode().IsDir() {
			hdr.Typeflag = tar.TypeDir
		} else if info.Mode().IsRegular() {
			hdr.Typeflag = tar.TypeReg
		} else {
			return fmt.Errorf("not implemented archiving file type %s (%s)", info.Mode(), info.Name())
		}

		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("failed to write tar header: %w", err)
		}
		if !info.IsDir() {
			f, err := os.Open(filepath.Clean(fp))
			if err != nil {
				return err
			}
			if _, err := io.Copy(tw, f); err != nil {
				return fmt.Errorf("failed to read file into the tar: %w", err)
			}
			err = f.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}

	if pathInfo.IsDir() {
		err := filepath.Walk(path, func(fp string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(path, fp)
			if err != nil {
				return fmt.Errorf("failed to calculate relative path: %w", err)
			}

			hdr := &tar.Header{
				Name:   filepath.Join(targetPath, filepath.ToSlash(rel)),
				Format: tar.FormatPAX,
				Mode:   int64(info.Mode().Perm()),
			}
			if uid != -1 {
				hdr.Uid = uid
			}
			if gid != -1 {
				hdr.Gid = gid
			}

			if info.Mode()&os.ModeSetuid != 0 {
				hdr.Mode |= c_ISUID
			}
			if info.Mode()&os.ModeSetgid != 0 {
				hdr.Mode |= c_ISGID
			}
			if info.Mode()&os.ModeSticky != 0 {
				hdr.Mode |= c_ISVTX
			}

			if err := processPaths(hdr, info, fp); err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to scan files: %w", err)
		}

	} else {
		base := filepath.Base(path)
		hdr := &tar.Header{
			Name:   filepath.Join(targetPath, filepath.ToSlash(base)),
			Format: tar.FormatPAX,
			Mode:   int64(pathInfo.Mode().Perm()),
		}
		if uid != -1 { // uid was specified in the input param
			hdr.Uid = uid
		}
		if gid != -1 { // gid was specified in the input param
			hdr.Gid = gid
		}

		if pathInfo.Mode()&os.ModeSetuid != 0 {
			hdr.Mode |= c_ISUID
		}
		if pathInfo.Mode()&os.ModeSetgid != 0 {
			hdr.Mode |= c_ISGID
		}
		if pathInfo.Mode()&os.ModeSticky != 0 {
			hdr.Mode |= c_ISVTX
		}

		if err := processPaths(hdr, pathInfo, path); err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("failed to finish tar: %w", err)
	}

	opener := func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(b.Bytes())), nil
	}
	return tarball.LayerFromOpener(opener)
}
