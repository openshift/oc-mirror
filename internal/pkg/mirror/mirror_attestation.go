package mirror

import (
	"context"
	"fmt"

	"go.podman.io/image/v5/copy"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/types"
)

// filterAttestationPlatforms inspects a source manifest list and returns only
// the real platforms when attestation entries are present. Attestation manifests
// use platform {architecture: "unknown", os: "unknown"} and cannot be copied
// from proxy registries like registry.connect.redhat.com.
//
// Returns (nil, nil) if the source is not a manifest list or contains no
// attestation entries — callers should preserve the original copy behavior.
func filterAttestationPlatforms(ctx context.Context, srcRef types.ImageReference, sysCtx *types.SystemContext) ([]copy.InstancePlatformFilter, error) {
	src, err := srcRef.NewImageSource(ctx, sysCtx)
	if err != nil {
		return nil, fmt.Errorf("opening image source for attestation check: %w", err)
	}
	defer src.Close()

	manifestBytes, mimeType, err := src.GetManifest(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("reading manifest for attestation check: %w", err)
	}

	if !manifest.MIMETypeIsMultiImage(mimeType) {
		return nil, nil
	}

	list, err := manifest.ListFromBlob(manifestBytes, mimeType)
	if err != nil {
		return nil, fmt.Errorf("parsing manifest list for attestation check: %w", err)
	}

	hasAttestation := false
	var platforms []copy.InstancePlatformFilter

	for _, d := range list.Instances() {
		info, err := list.Instance(d)
		if err != nil {
			continue
		}

		if isAttestationManifest(info) {
			hasAttestation = true
			continue
		}

		p := info.ReadOnly.Platform
		if p != nil {
			platforms = append(platforms, copy.InstancePlatformFilter{
				OS:           p.OS,
				Architecture: p.Architecture,
			})
		}
	}

	if !hasAttestation {
		return nil, nil
	}

	return platforms, nil
}

// isAttestationManifest returns true if a manifest list entry is an attestation
// manifest — identified by the "vnd.docker.reference.type" annotation or by
// having an unknown/unknown platform (common for buildx SLSA provenance).
func isAttestationManifest(info manifest.ListUpdate) bool {
	if info.ReadOnly.Annotations["vnd.docker.reference.type"] == "attestation-manifest" {
		return true
	}
	p := info.ReadOnly.Platform
	return p != nil && p.OS == "unknown" && p.Architecture == "unknown"
}
