package archive

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	digest "github.com/opencontainers/go-digest"

	"github.com/openshift/oc-mirror/v2/internal/pkg/errortype"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type ImageBlobGatherer struct {
	opts *mirror.CopyOptions
}

type internalImageBlobGatherer struct {
	imgRef         string
	sourceCtx      *types.SystemContext
	manifestBytes  []byte
	mimeType       string
	digest         digest.Digest
	copySignatures bool
}

func NewImageBlobGatherer(opts *mirror.CopyOptions) *ImageBlobGatherer {
	return &ImageBlobGatherer{
		opts: opts,
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

	manifestBytes, mime, err := imageManifest(ctx, sourceCtx, imgRef, nil)
	if err != nil {
		return nil, err
	}

	digest, err := manifest.Digest(manifestBytes)
	if err != nil {
		return nil, fmt.Errorf("error to get the digest of the image manifest %w", err)
	}

	inImageBlogGather := internalImageBlobGatherer{imgRef: imgRef, sourceCtx: sourceCtx, manifestBytes: manifestBytes, mimeType: mime, digest: digest, copySignatures: !o.opts.RemoveSignatures}

	if manifest.MIMETypeIsMultiImage(mime) {
		return multiArchBlobs(ctx, inImageBlogGather)
	} else {
		return singleArchBlobs(ctx, inImageBlogGather)
	}
}

// imageManifest returns the container image manifest content with its type.
func imageManifest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string, instanceDigest *digest.Digest) ([]byte, string, error) {
	srcRef, err := alltransports.ParseImageName(imgRef)
	if err != nil {
		return nil, "", fmt.Errorf("invalid source name %s: %w", imgRef, err)
	}

	img, err := srcRef.NewImageSource(ctx, sourceCtx)
	if err != nil {
		return nil, "", fmt.Errorf("error when creating a new image source %w", err)
	}
	defer img.Close()

	bytesManifest, mime, err := img.GetManifest(ctx, instanceDigest)
	if err != nil {
		return nil, "", fmt.Errorf("error to get the image manifest and mime type %w", err)
	}

	return bytesManifest, mime, nil
}

// multiArchBlobs returns the blobs of all architectures (including signature blobs if they exists).
func multiArchBlobs(ctx context.Context, in internalImageBlobGatherer) (map[string]struct{}, error) {
	var sigErrors []error

	blobs := make(map[string]struct{})

	blobs[in.digest.String()] = struct{}{}

	manifestList, err := manifest.ListFromBlob(in.manifestBytes, in.mimeType)
	if err != nil {
		return nil, fmt.Errorf("error to get the manifest list %w", err)
	}

	if in.copySignatures {
		sigBlobs, err := imageSignatureBlobs(ctx, in)
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

		singleIn.manifestBytes, singleIn.mimeType, err = imageManifest(ctx, in.sourceCtx, in.imgRef, &digest)
		if err != nil {
			return nil, err
		}
		singleIn.digest = digest

		singleArchBlobs, err := singleArchBlobs(ctx, singleIn)
		if err != nil {
			if errors.As(err, &errortype.SignatureBlobGathererError{}) {
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
func singleArchBlobs(ctx context.Context, in internalImageBlobGatherer) (map[string]struct{}, error) {
	var err error

	blobs := make(map[string]struct{})

	blobs[in.digest.String()] = struct{}{}

	manifestBlobs, err := imageBlobs(in.manifestBytes, in.mimeType)
	if err != nil {
		return nil, err
	}

	var sigBlobs []string
	if in.copySignatures {
		sigBlobs, err = imageSignatureBlobs(ctx, in)
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

// TODO investigate why even without config (registries.d) for registry.redhat.io (which means also no use-sigstore-attachments: true)
// the operator images are being mirrored with their sigstore signatures

// TODO check why the error below is happening
// 2025/04/01 19:14:32  [ERROR]  : [Worker] error mirroring image docker://registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:4a2324acaea757bae3b01b1aec59f49f4dd79bd1868e69d3418d57c210a6dfd9 (Operator bundles: [aws-load-balancer-operator.v1.2.0] - Operators: [aws-load-balancer-operator]) error: copying image 1/4 from manifest list: reading signatures: server provided 128 signatures, assuming that's unreasonable and a server error
// 2025/04/01 19:14:32  [ERROR]  : [Worker] error mirroring image docker://registry.redhat.io/albo/aws-load-balancer-operator-bundle@sha256:e371c45e259eaf54d79b1bfc6c47b1093d87965a8d6076205d4860047146ed43 error: skipping operator bundle docker://registry.redhat.io/albo/aws-load-balancer-operator-bundle@sha256:e371c45e259eaf54d79b1bfc6c47b1093d87965a8d6076205d4860047146ed43 because one of its related images failed to mirror

// TODO check why in m2d/d2m the signatures of the catalog image are being mirrored even after the rebuild

// imageSignatureBlobs returns the blobs of container image which is a signature.
func imageSignatureBlobs(ctx context.Context, in internalImageBlobGatherer) ([]string, error) {
	var ref image.ImageSpec
	tag, err := sigstoreAttachmentTag(in.digest)
	if err != nil {
		return nil, errortype.SignatureBlobGathererError{SigError: err}
	}

	if ref, err = image.ParseRef(in.imgRef); err != nil {
		return nil, errortype.SignatureBlobGathererError{SigError: err}
	}
	ref = ref.SetTag(tag)

	manifestBytes, mime, err := imageManifest(ctx, in.sourceCtx, ref.ReferenceWithTransport, nil)
	if err != nil {
		return nil, errortype.SignatureBlobGathererError{SigError: err}
	}

	signatureDigest, err := manifest.Digest(manifestBytes)
	if err != nil {
		return nil, errortype.SignatureBlobGathererError{SigError: fmt.Errorf("error to get the digest of the signature manifest %w", err)}
	}

	sigBlobs, err := imageBlobs(manifestBytes, mime)
	if err != nil {
		return nil, errortype.SignatureBlobGathererError{SigError: err}
	}

	sigBlobs = append(sigBlobs, signatureDigest.String())

	return sigBlobs, nil
}

// sigstoreAttachmentTag returns a sigstore attachment tag for the specified digest.
func sigstoreAttachmentTag(d digest.Digest) (string, error) {
	if err := d.Validate(); err != nil {
		return "", fmt.Errorf("invalid digest %w", err)
	}
	return strings.Replace(d.String(), ":", "-", 1) + ".sig", nil
}
