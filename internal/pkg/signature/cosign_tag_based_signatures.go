package signature

import (
	"context"
	"fmt"
	"strings"

	"github.com/opencontainers/go-digest"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/types"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	ocmirrormanifest "github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type SignatureHandler struct {
	opts             *mirror.CopyOptions
	log              clog.PluggableLoggerInterface
	ocmirrormanifest ocmirrormanifest.ManifestInterface
	destination      bool
}

// NewForDestination inspects mirrored images using destination credentials and
// TLS settings, for example when generating deletion metadata after m2m.
func NewForDestination(opts *mirror.CopyOptions, log clog.PluggableLoggerInterface) *SignatureHandler {
	handler := New(opts, log)
	handler.destination = true
	return handler
}

func New(opts *mirror.CopyOptions, log clog.PluggableLoggerInterface) *SignatureHandler {
	return &SignatureHandler{
		opts:             opts,
		log:              log,
		ocmirrormanifest: ocmirrormanifest.New(log),
	}
}

// SigstoreAttachmentTag returns a sigstore attachment tag for the specified digest.
func SigstoreAttachmentTag(d digest.Digest) (string, error) {
	if err := d.Validate(); err != nil {
		return "", fmt.Errorf("invalid digest %w", err)
	}
	return strings.Replace(d.String(), ":", "-", 1) + ".sig", nil
}

// GetSignatureTag returns the signature tag for the given image reference (single or multi arch).
func (o *SignatureHandler) GetSignatureTag(ctx context.Context, imgRef string) ([]string, error) {
	var sourceCtx *types.SystemContext
	var err error
	if o.destination {
		sourceCtx, err = o.opts.DestImage.NewSystemContext()
	} else {
		sourceCtx, err = o.opts.SrcImage.NewSystemContext()
	}
	if err != nil {
		return nil, err
	}
	if !o.destination {
		sourceCtx.DockerInsecureSkipTLSVerify = types.NewOptionalBool(true)
	}

	bytesManifest, mime, err := o.ocmirrormanifest.ImageManifest(ctx, sourceCtx, imgRef, nil)
	if err != nil {
		return nil, fmt.Errorf("error to get the image manifest and mime type %w", err)
	}

	digest, err := manifest.Digest(bytesManifest)
	if err != nil {
		return nil, fmt.Errorf("error to get the digest of the image manifest %w", err)
	}

	if manifest.MIMETypeIsMultiImage(mime) {
		return o.multiArchSigTags(bytesManifest, mime, digest)
	}
	return o.singleArchSigTags(digest)
}

func (o *SignatureHandler) multiArchSigTags(bytesManifest []byte, mime string, digest digest.Digest) ([]string, error) {
	tags := []string{}
	sigTags, err := o.singleArchSigTags(digest)
	if err != nil {
		return nil, err
	}
	tags = append(tags, sigTags...)

	manifestList, err := manifest.ListFromBlob(bytesManifest, mime)
	if err != nil {
		return nil, fmt.Errorf("error to get the manifest list %w", err)
	}

	digests := manifestList.Instances()
	for _, digest := range digests {
		sigTags, err := o.singleArchSigTags(digest)
		if err != nil {
			return nil, err
		}
		tags = append(tags, sigTags...)
	}
	return tags, nil
}

func (o *SignatureHandler) singleArchSigTags(digest digest.Digest) ([]string, error) {
	tags := []string{}
	tag, err := SigstoreAttachmentTag(digest)
	if err != nil {
		return nil, err
	}
	tags = append(tags, tag)
	return tags, nil
}
