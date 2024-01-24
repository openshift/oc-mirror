package imagebuilder

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/match"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

// ImageBuilder use an OCI workspace to add layers and change configuration to images.
type ImageBuilder struct {
	NameOpts   []name.Option
	RemoteOpts []remote.Option
	Logger     log.PluggableLoggerInterface
}

// ErrInvalidReference is returned the target reference is a digest.
type ErrInvalidReference struct {
	image string
}

func (e ErrInvalidReference) Error() string {
	return fmt.Sprintf("target reference %q must have a tag reference", e.image)
}

// NewImageBuilder creates a new instance of an ImageBuilder.
func NewBuilder(logger log.PluggableLoggerInterface, opts mirror.CopyOptions) ImageBuilderInterface {
	// preparing name options for pulling the ubi9 image:
	// - no need to set defaultRegistry because we are using a fully qualified image name
	nameOptions := []name.Option{
		name.StrictValidation,
	}
	remoteOptions := []remote.Option{
		remote.WithAuthFromKeychain(authn.DefaultKeychain), // this will try to find .docker/config first, $XDG_RUNTIME_DIR/containers/auth.json second
		remote.WithContext(context.TODO()),
		// doesn't seem possible to use registries.conf here.
	}
	if !opts.Global.TlsVerify {
		remoteOptions = append(remoteOptions, remote.WithTransport(remote.DefaultTransport))
	} else {
		nameOptions = append(nameOptions, name.Insecure)
		// create our own roundTripper to pass insecure=true
		insecureRoundTripper := createInsecureRoundTripper()
		remoteOptions = append(remoteOptions, remote.WithTransport(insecureRoundTripper))
	}

	return &ImageBuilder{
		NameOpts:   nameOptions,
		RemoteOpts: remoteOptions,
		Logger:     logger,
	}
}

func createInsecureRoundTripper() http.RoundTripper {
	// create a custom transport that will allow us to use a custom TLS config
	// this will allow us to disable TLS verification
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			// By default, we wrap the transport in retries, so reduce the
			// default dial timeout to 5s to avoid 5x 30s of connection
			// timeouts when doing the "ping" on certain http registries.
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS12,
		},
	}
}

// Run modifies and pushes the catalog image existing in an OCI layout. The image configuration will be updated
// with the required labels and any provided layers will be appended.
// # Arguments
// • ctx: a cancellation context
// • targetRef: a docker image reference
// • layoutPath: an OCI image layout path
// • update: an optional function that allows callers to modify the *v1.ConfigFile if necessary
// • layers: zero or more layers to add to the images discovered during processing
// # Returns
// error: non-nil on error, nil otherwise

func (b *ImageBuilder) BuildAndPush(ctx context.Context, targetRef string, layoutPath layout.Path, cmd []string, layers ...v1.Layer) error {

	var v2format bool

	b.RemoteOpts = append(b.RemoteOpts, remote.WithContext(ctx))

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
	resultIdx, err := b.ProcessImageIndex(ctx, idx, &v2format, cmd, targetRef, layers...)
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

// ProcessImageIndex is a recursive helper function that allows for traversal of the hierarchy of
// parent/child indexes that can exist for a multi arch image. There's always
// at least one index at the root since this is an OCI layout that we're dealing with.
// In theory there can be "infinite levels" of "index indirection" for multi arch images, but typically
// its only two levels deep (i.e. index.json itself which is level one, and the manifest list
// defined in the blobs directory, which is level two).

// Each image that is encountered is updated using the update function (if provided) and whatever layers are provided.

// # Arguments
// • ctx: a cancellation context
// • idx: the "current" image index for this stage of recursion
// • v2format: a boolean used to keep track of the type of image we're dealing with. false means OCI media types
// should be used and true means docker v2s2 media types should be used
// • update: an optional function that allows callers to modify the *v1.ConfigFile if necessary
// • targetRef: the docker image reference, which is only used for error reporting in this function
// • layers: zero or more layers to add to the images discovered during processing

// # Returns
// • v1.ImageIndex: The resulting image index after processing has completed. Will be nil if an error occurs, otherwise non-nil.
// • error: non-nil if an error occurs, nil otherwise
func (b *ImageBuilder) ProcessImageIndex(ctx context.Context, idx v1.ImageIndex, v2format *bool, cmd []string, targetRef string, layers ...v1.Layer) (v1.ImageIndex, error) {
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
			processedIdx, err := b.ProcessImageIndex(ctx, innerIdx, v2format, cmd, targetRef, layers...)
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

		if len(cmd) > 0 {
			// Update image config
			cfg, err := img.ConfigFile()
			if err != nil {
				return nil, err
			}
			cfg.Config.Cmd = cmd
			cfg.Author = "oc-mirror"
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

// SaveImageLayoutToDir saves the image layout of the specified image reference to the specified directory.
// It returns the path to the saved layout and any error encountered during the process.
func (b *ImageBuilder) SaveImageLayoutToDir(ctx context.Context, imgRef string, layoutDir string) (layout.Path, error) {
	b.RemoteOpts = append(b.RemoteOpts, remote.WithContext(ctx))

	ref, err := name.ParseReference(imgRef, b.NameOpts...)
	if err != nil {
		return "nil", err
	}
	idx, err := remote.Index(ref, b.RemoteOpts...)
	if err != nil {
		return "", err
	}
	return layout.Write(layoutDir, idx)
}

func LayerFromGzipByteArray(content []byte, outputFile string, contentPrefixDir string, mod int, uid, gid int) (v1.Layer, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	f, err := os.Create(outputFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tarWriter := tar.NewWriter(f)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		header.Name = filepath.Join(contentPrefixDir, header.Name)
		header.Uid = uid
		header.Gid = gid

		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, err
		}

		if _, err := io.Copy(tarWriter, tarReader); err != nil {
			return nil, err
		}
	}

	if err := tarWriter.Close(); err != nil {
		return nil, err
	}

	if err := os.Chmod(outputFile, os.FileMode(mod)); err != nil {
		return nil, err
	}

	layerOptions := []tarball.LayerOption{}
	layer, err := tarball.LayerFromFile(outputFile, layerOptions...)
	if err != nil {
		return nil, err
	}
	return layer, nil
}
