package archive

import (
	"context"
	"errors"
	"fmt"
	"maps"

	digest "github.com/opencontainers/go-digest"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	ocmirrormanifest "github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/signature"
)

type ImageBlobGatherer struct {
	opts             *mirror.CopyOptions
	log              clog.PluggableLoggerInterface
	ocmirrormanifest ocmirrormanifest.ManifestInterface
}

type internalImageBlobGatherer struct {
	imgRef         string
	sourceCtx      *types.SystemContext
	manifestBytes  []byte
	mimeType       string
	digest         digest.Digest
	copySignatures bool
}

func NewImageBlobGatherer(opts *mirror.CopyOptions, log clog.PluggableLoggerInterface) *ImageBlobGatherer {
	return &ImageBlobGatherer{
		opts:             opts,
		log:              log,
		ocmirrormanifest: ocmirrormanifest.New(log),
	}
}

// GatherBlobs returns all container image blobs (including signature blobs if they exists).
func (o *ImageBlobGatherer) GatherBlobs(ctx context.Context, imgRef string) (blobs map[string]struct{}, retErr error) {
	// we are always gathering blobs from the local cache registry - skipping tls verification
	sourceCtx, err := o.opts.SrcImage.NewSystemContext()
	if err != nil {
		return nil, err
	}
	sourceCtx.DockerInsecureSkipTLSVerify = types.NewOptionalBool(true)

	manifestBytes, mime, err := o.ocmirrormanifest.ImageManifest(ctx, sourceCtx, imgRef, nil)
	if err != nil {
		return nil, err
	}

	digest, err := manifest.Digest(manifestBytes)
	if err != nil {
		return nil, fmt.Errorf("error to get the digest of the image manifest %w", err)
	}

	inImageBlogGather := internalImageBlobGatherer{imgRef: imgRef, sourceCtx: sourceCtx, manifestBytes: manifestBytes, mimeType: mime, digest: digest, copySignatures: !o.opts.RemoveSignatures}

	if manifest.MIMETypeIsMultiImage(mime) {
		return o.multiArchBlobs(ctx, inImageBlogGather)
	} else {
		return o.singleArchBlobs(ctx, inImageBlogGather)
	}
}

// multiArchBlobs returns the blobs of all architectures (including signature blobs if they exists).
func (o *ImageBlobGatherer) multiArchBlobs(ctx context.Context, in internalImageBlobGatherer) (map[string]struct{}, error) {
	var sigErrors []error

	blobs := make(map[string]struct{})

	blobs[in.digest.String()] = struct{}{}

	manifestList, err := manifest.ListFromBlob(in.manifestBytes, in.mimeType)
	if err != nil {
		return nil, fmt.Errorf("error to get the manifest list %w", err)
	}

	if in.copySignatures {
		sigBlobs, err := o.imageSignatureBlobs(ctx, in)
		if err == nil {
			for _, digest := range sigBlobs {
				blobs[digest] = struct{}{}
			}
		} else {
			sigErrors = append(sigErrors, err)
		}

	}

	digests := manifestList.Instances()
	for _, digest := range digests {
		blobs[digest.String()] = struct{}{}
		singleIn := in

		singleIn.manifestBytes, singleIn.mimeType, err = o.ocmirrormanifest.ImageManifest(ctx, in.sourceCtx, in.imgRef, &digest)
		if err != nil {
			return nil, err
		}
		singleIn.digest = digest

		singleArchBlobs, err := o.singleArchBlobs(ctx, singleIn)
		if err != nil {
			var sigErr *SignatureBlobGathererError
			if errors.As(err, &sigErr) {
				sigErrors = append(sigErrors, err)
			} else {
				return nil, err
			}
		}

		maps.Copy(blobs, singleArchBlobs)
	}

	return blobs, errors.Join(sigErrors...)
}

// singleArchBlobs returns the blobs of single architecture (including signature blobs if they exists).
func (o *ImageBlobGatherer) singleArchBlobs(ctx context.Context, in internalImageBlobGatherer) (map[string]struct{}, error) {
	var err error

	blobs := make(map[string]struct{})

	blobs[in.digest.String()] = struct{}{}

	manifestBlobs, err := imageBlobs(in.manifestBytes, in.mimeType)
	if err != nil {
		return nil, err
	}

	var sigBlobs []string
	if in.copySignatures {
		sigBlobs, err = o.imageSignatureBlobs(ctx, in)
		if err == nil {
			manifestBlobs = append(manifestBlobs, sigBlobs...)
		}
	}

	for _, digest := range manifestBlobs {
		blobs[digest] = struct{}{}
	}

	return blobs, err
}

// imageBlobs returns the blobs of a container image which is not a signature.
func imageBlobs(manifestBytes []byte, mimeType string) ([]string, error) {
	blobs := []string{}
	singleArchManifest, err := manifest.FromBlob(manifestBytes, mimeType)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling manifest: %w", err)
	}
	for _, layer := range singleArchManifest.LayerInfos() {
		blobs = append(blobs, layer.Digest.String())
	}
	blobs = append(blobs, singleArchManifest.ConfigInfo().Digest.String())
	return blobs, nil
}

// imageSignatureBlobs returns the blobs of container image which is a signature.
func (o *ImageBlobGatherer) imageSignatureBlobs(ctx context.Context, in internalImageBlobGatherer) ([]string, error) {
	var ref image.ImageSpec
	tag, err := signature.SigstoreAttachmentTag(in.digest)
	if err != nil {
		return nil, &SignatureBlobGathererError{SigError: err}
	}

	if ref, err = image.ParseRef(in.imgRef); err != nil {
		return nil, &SignatureBlobGathererError{SigError: err}
	}
	ref = ref.SetTag(tag)

	manifestBytes, mime, err := o.ocmirrormanifest.ImageManifest(ctx, in.sourceCtx, ref.ReferenceWithTransport, nil)
	if err != nil {
		return nil, &SignatureBlobGathererError{SigError: err}
	}

	signatureDigest, err := manifest.Digest(manifestBytes)
	if err != nil {
		return nil, &SignatureBlobGathererError{SigError: fmt.Errorf("error to get the digest of the signature manifest %w", err)}
	}

	sigBlobs, err := imageBlobs(manifestBytes, mime)
	if err != nil {
		return nil, &SignatureBlobGathererError{SigError: err}
	}

	sigBlobs = append(sigBlobs, signatureDigest.String())

	return sigBlobs, nil
}
