package archive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	digest "github.com/opencontainers/go-digest"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/types"
	"k8s.io/apimachinery/pkg/util/sets"

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
	imgRef           string
	sourceCtx        *types.SystemContext
	manifestBytes    []byte
	mimeType         string
	digest           digest.Digest
	copySignatures   bool
	allowedPlatforms []string // non-empty = only these os/arch pairs were mirrored (sparse)
}

func NewImageBlobGatherer(opts *mirror.CopyOptions, log clog.PluggableLoggerInterface) *ImageBlobGatherer {
	return &ImageBlobGatherer{
		opts:             opts,
		log:              log,
		ocmirrormanifest: ocmirrormanifest.New(log),
	}
}

// GatherBlobs returns all container image blobs (including signature blobs if they exist).
// See BlobsGatherer.GatherBlobs for the allowedPlatforms semantics.
func (o *ImageBlobGatherer) GatherBlobs(ctx context.Context, imgRef string, allowedPlatforms []string) (blobs sets.Set[string], retErr error) {
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

	inImageBlogGather := internalImageBlobGatherer{imgRef: imgRef, sourceCtx: sourceCtx, manifestBytes: manifestBytes, mimeType: mime, digest: digest, copySignatures: !o.opts.RemoveSignatures, allowedPlatforms: allowedPlatforms}

	if manifest.MIMETypeIsMultiImage(mime) {
		return o.multiArchBlobs(ctx, inImageBlogGather)
	} else {
		return o.singleArchBlobs(ctx, inImageBlogGather)
	}
}

// multiArchBlobs returns the blobs of all architectures (including signature blobs if they exists).
func (o *ImageBlobGatherer) multiArchBlobs(ctx context.Context, in internalImageBlobGatherer) (sets.Set[string], error) {
	var sigErrors []error

	blobs := sets.New(in.digest.String())

	manifestList, err := manifest.ListFromBlob(in.manifestBytes, in.mimeType)
	if err != nil {
		return nil, fmt.Errorf("error to get the manifest list %w", err)
	}

	if in.copySignatures {
		// OCPBUGS-87160:
		// In the beginning, cosign did not support signing manifest lists.
		// Also podman/clusterimagepolicy only verifies the single arch manifest signatures.
		// Because of that, ART only signs the single arch manifests not the manifest list itself.
		if sigBlobs, err := o.imageSignatureBlobs(ctx, in); err == nil {
			blobs.Insert(sigBlobs...)
		} else {
			o.log.Debug("Skip signature gathering for manifest list %q: %s", in.digest.String(), err.Error())
		}
	}

	digests := manifestList.Instances()
	for _, digest := range digests {
		singleIn := in

		singleIn.manifestBytes, singleIn.mimeType, err = o.ocmirrormanifest.ImageManifest(ctx, in.sourceCtx, in.imgRef, &digest)
		if err != nil {
			if strings.Contains(err.Error(), "manifest unknown") && len(in.allowedPlatforms) > 0 {
				platform := platformForDigest(in.manifestBytes, digest.String())
				if platform != "" && !platformAllowed(platform, in.allowedPlatforms) {
					// Platform was intentionally not mirrored (sparse manifest filtering).
					// Do not add its digest to blobs — the blob file does not exist locally.
					o.log.Debug("Skipping absent platform %q for digest %s (not in allowed platforms)", platform, digest.String())
					continue
				}
			}
			return nil, err
		}
		// Only insert the platform digest after confirming the manifest is present.
		blobs.Insert(digest.String())
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

		blobs = blobs.Union(singleArchBlobs)
	}

	return blobs, errors.Join(sigErrors...)
}

// singleArchBlobs returns the blobs of single architecture (including signature blobs if they exists).
func (o *ImageBlobGatherer) singleArchBlobs(ctx context.Context, in internalImageBlobGatherer) (sets.Set[string], error) {
	var err error

	blobs := sets.New(in.digest.String())

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

	blobs.Insert(manifestBlobs...)

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

// platformForDigest returns the "os/arch" string for the manifest list entry
// matching digestStr, or an empty string if not found.
func platformForDigest(manifestBytes []byte, digestStr string) string {
	var ml struct {
		Manifests []struct {
			Digest   string `json:"digest"`
			Platform struct {
				OS           string `json:"os"`
				Architecture string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(manifestBytes, &ml); err != nil {
		return ""
	}
	for _, m := range ml.Manifests {
		if m.Digest == digestStr {
			return m.Platform.OS + "/" + m.Platform.Architecture
		}
	}
	return ""
}

// platformAllowed reports whether platform is in allowedPlatforms.
func platformAllowed(platform string, allowedPlatforms []string) bool {
	for _, allowed := range allowedPlatforms {
		if platform == allowed {
			return true
		}
	}
	return false
}
