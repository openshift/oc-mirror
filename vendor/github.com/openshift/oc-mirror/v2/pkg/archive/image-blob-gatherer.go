package archive

import (
	"context"
	"fmt"
	"strconv"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

type ImageBlobGatherer struct {
	ctx  context.Context
	opts *mirror.CopyOptions
}

func NewImageBlobGatherer(ctx context.Context, opts *mirror.CopyOptions) *ImageBlobGatherer {
	return &ImageBlobGatherer{
		ctx:  ctx,
		opts: opts,
	}
}
func (o *ImageBlobGatherer) GatherBlobs(imgRef string) (blobs []string, retErr error) {
	blobs = []string{}
	o.opts.DeprecatedTLSVerify.WarnIfUsed([]string{"--src-tls-verify", "--dest-tls-verify"})
	// o.opts.All = true
	o.opts.RemoveSignatures, _ = strconv.ParseBool("true")

	if err := mirror.ReexecIfNecessaryForImages([]string{imgRef}...); err != nil {
		return blobs, err
	}

	// TODO should we verify signatures while gathering blobs?
	// More broadly, should we include anything in the archive to
	// help with signature verification after the archive is untarred
	// inside the enclave?

	srcRef, err := alltransports.ParseImageName(imgRef)
	if err != nil {
		return blobs, fmt.Errorf("invalid source name %s: %v", imgRef, err)
	}
	sourceCtx, err := o.opts.SrcImage.NewSystemContext()
	if err != nil {
		return blobs, err
	}

	img, err := srcRef.NewImageSource(o.ctx, sourceCtx)
	if err != nil {
		return blobs, err
	}

	manifestBytes, mime, err := img.GetManifest(o.ctx, nil)
	if err != nil {
		return blobs, err
	}

	digest, err := manifest.Digest(manifestBytes)
	if err != nil {
		return blobs, err
	}
	blobs = append(blobs, digest.String())

	if manifest.MIMETypeIsMultiImage(mime) {
		manifestList, err := manifest.ListFromBlob(manifestBytes, mime)
		if err != nil {
			return blobs, err
		}
		instances := manifestList.Instances()
		for _, digest := range instances {
			blobs = append(blobs, digest.String())
			singleArchManifest, singleArchMime, err := img.GetManifest(o.ctx, &digest)
			if err != nil {
				return blobs, err
			}
			singleArchBlobs, err := o.getBlobsOfManifest(singleArchManifest, singleArchMime)
			if err != nil {
				return blobs, err
			}
			blobs = append(blobs, singleArchBlobs...)
			if err != nil {
				return blobs, err
			}
		}
	} else {

		manifestBlobs, err := o.getBlobsOfManifest(manifestBytes, mime)
		if err != nil {
			return blobs, err
		}
		blobs = append(blobs, manifestBlobs...)
	}
	return blobs, nil
}

func (o *ImageBlobGatherer) getBlobsOfManifest(manifestBytes []byte, mimeType string) ([]string, error) {
	blobs := []string{}
	s2v2man, err := manifest.FromBlob(manifestBytes, mimeType)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling manifest: %v", err)
	}
	for _, layer := range s2v2man.LayerInfos() {
		blobs = append(blobs, layer.Digest.String())
	}
	blobs = append(blobs, s2v2man.ConfigInfo().Digest.String())
	return blobs, nil
}
