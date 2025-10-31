package image

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/pkg/cli/environment"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	imgreference "github.com/openshift/library-go/pkg/image/reference"
	"k8s.io/klog"
)

func ResolveToPin(ctx context.Context, sourceCtx *types.SystemContext, unresolvedImage string) (string, error) {
	// Add the digest to the Reference to use it's Stringer implementation
	// to get the full image pin.
	ref, err := imgreference.Parse(unresolvedImage)
	if err != nil {
		return "", err
	}
	ref = ref.DockerClientDefaults()
	srcRef, err := alltransports.ParseImageName("docker://" + ref.String())
	if err != nil {
		return "", fmt.Errorf("invalid source name %s: %v", ref.String(), err)
	}

	img, err := srcRef.NewImageSource(ctx, sourceCtx)
	if err != nil {
		return "", err
	}

	manifestBytes, _, err := img.GetManifest(ctx, nil)
	if err != nil {
		return "", err
	}

	digest, err := manifest.Digest(manifestBytes)
	if err != nil {
		return "", err
	}

	ref.ID = digest.String()
	return ref.String(), nil
}

func NewSystemContext(skipTLS bool, registriesConfigPath string) *types.SystemContext {
	skipTLSVerify := types.OptionalBoolFalse
	if skipTLS {
		skipTLSVerify = types.OptionalBoolTrue
	}
	ctx := &types.SystemContext{
		RegistriesDirPath:           "",
		ArchitectureChoice:          "",
		OSChoice:                    "",
		VariantChoice:               "",
		BigFilesTemporaryDir:        "", //*globalArgs.cache + "/tmp",
		DockerInsecureSkipTLSVerify: skipTLSVerify,
	}
	if registriesConfigPath != "" {
		ctx.SystemRegistriesConfPath = registriesConfigPath
	} else {
		err := environment.UpdateRegistriesConf(ctx)
		if err != nil {
			// log and ignore
			klog.Warningf("unable to load registries.conf from environment variables: %v", err)

		}
	}
	return ctx
}

// IsImagePinned returns true if img looks canonical.
func IsImagePinned(img string) bool {
	return strings.Contains(img, "@")
}

// IsImageTagged returns true if img has a tag.
func IsImageTagged(img string) bool {
	return strings.Contains(img, ":")
}
