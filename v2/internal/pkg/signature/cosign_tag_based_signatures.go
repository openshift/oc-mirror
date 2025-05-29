package signature

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"

	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type SignatureHandler struct {
	opts *mirror.CopyOptions
}

func New(opts *mirror.CopyOptions) *SignatureHandler {
	return &SignatureHandler{
		opts: opts,
	}
}

// SigstoreAttachmentTag returns a sigstore attachment tag for the specified digest.
func SigstoreAttachmentTag(d digest.Digest) (string, error) {
	if err := d.Validate(); err != nil {
		return "", fmt.Errorf("invalid digest %w", err)
	}
	return strings.Replace(d.String(), ":", "-", 1) + ".sig", nil
}

func (o *SignatureHandler) GetSignatureTag(ctx context.Context, imgRef string) ([]string, error) {
	tags := []string{}

	sourceCtx, err := o.opts.SrcImage.NewSystemContext()
	if err != nil {
		return nil, err
	}
	sourceCtx.DockerInsecureSkipTLSVerify = types.NewOptionalBool(true)

	srcRef, err := alltransports.ParseImageName(imgRef)
	if err != nil {
		return nil, fmt.Errorf("invalid source name %s: %w", imgRef, err)
	}

	img, err := srcRef.NewImageSource(ctx, sourceCtx)
	if err != nil {
		return nil, fmt.Errorf("error when creating a new image source: %w", err)
	}
	defer img.Close()

	bytesManifest, mime, err := img.GetManifest(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("error to get the image manifest and mime type %w", err)
	}

	digest, err := manifest.Digest(bytesManifest)
	if err != nil {
		return nil, fmt.Errorf("error to get the digest of the image manifest %w", err)
	}

	if manifest.MIMETypeIsMultiImage(mime) {
		tag, err := SigstoreAttachmentTag(digest)
		if err != nil {
			return tags, err
		}
		tags = append(tags, tag)

		manifestList, err := manifest.ListFromBlob(bytesManifest, mime)
		if err != nil {
			return nil, fmt.Errorf("error to get the manifest list %w", err)
		}

		digests := manifestList.Instances()
		for _, digest := range digests {
			tag, err := SigstoreAttachmentTag(digest)
			if err != nil {
				return tags, err
			}
			tags = append(tags, tag)
		}

	} else {
		// single arch
		tag, err := SigstoreAttachmentTag(digest)
		if err != nil {
			return tags, err
		}
		tags = append(tags, tag)
	}

	return tags, nil
}
