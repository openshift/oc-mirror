package mirror

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/distribution/reference"
	"go.podman.io/common/pkg/retry"
	"go.podman.io/image/v5/copy"
	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/pkg/cli"
	"go.podman.io/image/v5/signature"
	"go.podman.io/image/v5/transports/alltransports"
	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/registriesd"
)

type Mode string

// MirrorInterface  used to mirror images with container/images (skopeo)
type MirrorInterface interface {
	Run(ctx context.Context, src, dest string, mode Mode, opts *CopyOptions) (retErr error)
	Check(ctx context.Context, image string, opts *CopyOptions, asCopySrc bool) (bool, error)
}

type MirrorCopyInterface interface {
	CopyImage(ctx context.Context, pc *signature.PolicyContext, destRef, srcRef types.ImageReference, opts *copy.Options) ([]byte, error)
}

type MirrorDeleteInterface interface {
	DeleteImage(ctx context.Context, image string, opts *CopyOptions) error
}

// Mirror
type Mirror struct {
	mc   MirrorCopyInterface
	md   MirrorDeleteInterface
	Mode string
}

type MirrorCopy struct{}
type MirrorDelete struct{}

// New returns new Mirror instance
func New(mc MirrorCopyInterface, md MirrorDeleteInterface) MirrorInterface {
	return &Mirror{mc: mc, md: md}
}

func NewMirrorCopy() MirrorCopyInterface {
	return &MirrorCopy{}
}

func NewMirrorDelete() MirrorDeleteInterface {
	return &MirrorDelete{}
}

// Run - method to copy images from source to destination
func (o *Mirror) Run(ctx context.Context, src, dest string, mode Mode, opts *CopyOptions) (retErr error) {
	if mode == DeleteMode {
		return o.delete(ctx, dest, opts)
	}
	return o.copy(ctx, src, dest, opts)
}

func (o *MirrorCopy) CopyImage(ctx context.Context, pc *signature.PolicyContext, destRef, srcRef types.ImageReference, co *copy.Options) ([]byte, error) {
	return copy.Image(ctx, pc, destRef, srcRef, co)
}

func (o *MirrorDelete) DeleteImage(ctx context.Context, image string, co *CopyOptions) error {
	return nil
}

// copy - copy images setup and execute
func (o *Mirror) copy(ctx context.Context, src, dest string, opts *CopyOptions) (retErr error) {
	if err := ReexecIfNecessaryForImages([]string{src, dest}...); err != nil {
		return err
	}

	policyContext, err := opts.Global.GetPolicyContext(Mode(o.Mode))
	if err != nil {
		return fmt.Errorf("error loading trust policy: %v", err)
	}
	defer func() {
		if err := policyContext.Destroy(); err != nil {
			retErr = NoteCloseFailure(retErr, "tearing down policy context", err)
		}
	}()

	srcRef, err := alltransports.ParseImageName(src)
	if err != nil {
		return fmt.Errorf("invalid source name %s: %v", src, err)
	}
	destRef, err := alltransports.ParseImageName(dest)
	if err != nil {
		return fmt.Errorf("invalid destination name %s: %v", dest, err)
	}

	sourceCtx, err := opts.SrcImage.NewSystemContext()
	if err != nil {
		return err
	}
	if strings.Contains(src, opts.LocalStorageFQDN) { // when copying from cache, use HTTP
		sourceCtx.DockerInsecureSkipTLSVerify = types.OptionalBoolTrue
	}

	if !opts.RemoveSignatures {
		sourceCtx.RegistriesDirPath = registriesd.GetWorkingDirRegistrydConfigPath(opts.SrcImage.global.WorkingDir)
	}

	destinationCtx, err := opts.DestImage.NewSystemContext()
	if err != nil {
		return err
	}

	if strings.Contains(dest, opts.LocalStorageFQDN) { // when copying to cache, use HTTP
		destinationCtx.DockerInsecureSkipTLSVerify = types.OptionalBoolTrue
	}

	if !opts.RemoveSignatures {
		destinationCtx.RegistriesDirPath = registriesd.GetWorkingDirRegistrydConfigPath(opts.DestImage.global.WorkingDir)
	}

	var manifestType string
	if len(opts.Format) > 0 {
		manifestType, err = ParseManifestFormat(opts.Format)
		if err != nil {
			return err
		}
	}

	imageListSelection, instancePlatforms, err := determinePlatformSelection(opts)
	if err != nil {
		return err
	}

	if len(opts.EncryptionKeys) > 0 && len(opts.DecryptionKeys) > 0 {
		return fmt.Errorf("--encryption-key and --decryption-key cannot be specified together")
	}

	// c/image/copy.Image does allow creating both simple signing and sigstore signatures simultaneously,
	// with independent passphrases, but that would make the CLI probably too confusing.
	// For now, use the passphrase with either, but only one of them.
	if opts.SignPassphraseFile != "" && opts.SignByFingerprint != "" && opts.SignBySigstorePrivateKey != "" {
		return fmt.Errorf("only one of --sign-by and sign-by-sigstore-private-key can be used with sign-passphrase-file")
	}
	var passphrase string
	if opts.SignPassphraseFile != "" {
		p, err := cli.ReadPassphraseFile(opts.SignPassphraseFile)
		if err != nil {
			return err
		}
		passphrase = p
	}

	// opts.signByFingerprint triggers a GPG-agent passphrase prompt, possibly using a more secure channel,
	// so we usually shouldn’t prompt ourselves if no passphrase was explicitly provided.
	var signIdentity reference.Named = nil
	if opts.SignIdentity != "" {
		signIdentity, err = reference.ParseNamed(opts.SignIdentity)
		if err != nil {
			return fmt.Errorf("could not parse --sign-identity: %v", err)
		}
	}

	// hard coded ReportWriter to io.Discard
	co := &copy.Options{
		RemoveSignatures:                 opts.RemoveSignatures,
		SignBy:                           opts.SignByFingerprint,
		SignPassphrase:                   passphrase,
		SignBySigstorePrivateKeyFile:     opts.SignBySigstorePrivateKey,
		SignSigstorePrivateKeyPassphrase: []byte(passphrase),
		SignIdentity:                     signIdentity,
		ReportWriter:                     io.Discard,
		SourceCtx:                        sourceCtx,
		DestinationCtx:                   destinationCtx,
		ForceManifestMIMEType:            manifestType,
		ImageListSelection:               imageListSelection,
		InstancePlatforms:                instancePlatforms,
		PreserveDigests:                  opts.PreserveDigests,
		MaxParallelDownloads:             opts.ParallelLayerImages,
	}

	if opts.Global.LogLevel == "debug" {
		co.ReportWriter = opts.Stdout
	}

	var retryOpts retry.Options
	if opts.RetryOpts != nil {
		retryOpts = *opts.RetryOpts
	}
	retryOpts.IsErrorRetryable = isErrorRetryable

	//nolint:wrapcheck // context will be added by the calling function
	return retry.IfNecessary(ctx, func() error {
		manifestBytes, err := o.mc.CopyImage(ctx, policyContext, destRef, srcRef, co)
		if err != nil {
			return err
		}
		if opts.DigestFile != "" {
			manifestDigest, err := manifest.Digest(manifestBytes)
			if err != nil {
				return err
			}
			if err = os.WriteFile(opts.DigestFile, []byte(manifestDigest.String()), 0644); err != nil {
				return fmt.Errorf("failed to write digest to file %q: %w", opts.DigestFile, err)
			}
		}
		return nil
	}, &retryOpts)
}

// Custom implementation to extend `containers/common/pkg/retry.retry`
func isErrorRetryable(err error) bool {
	var httpError docker.UnexpectedHTTPStatusError
	switch {
	case err == nil:
		return false
	case errors.Is(err, context.DeadlineExceeded):
		return true
	case errors.Is(err, context.Canceled):
		return false
	case errors.As(err, &httpError):
		// Retry on 500-504 server errors, they appear to be quite common in the field
		// We duplicate this here because older versions of oc-mirror cannot bump containers/common given Golang version restrictions
		if httpError.StatusCode >= http.StatusInternalServerError && httpError.StatusCode <= http.StatusGatewayTimeout {
			return true
		}
		return false
	default:
		// Delegate the remaining checks to containers/common
		return retry.IsErrorRetryable(err)
	}
}

// check exists - checks if image exists
func (o *Mirror) Check(ctx context.Context, image string, opts *CopyOptions, asCopySrc bool) (bool, error) {

	if err := ReexecIfNecessaryForImages([]string{image}...); err != nil {
		return false, err
	}

	imageRef, err := alltransports.ParseImageName(image)
	if err != nil {
		return false, fmt.Errorf("invalid source name %s: %v", image, err)
	}
	var sysCtx *types.SystemContext
	if asCopySrc {
		sysCtx, err = opts.SrcImage.NewSystemContext()
		if err != nil {
			return false, err
		}
	} else {
		sysCtx, err = opts.DestImage.NewSystemContext()
		if err != nil {
			return false, err
		}
	}

	ctx, cancel := opts.Global.CommandTimeoutContext()
	defer cancel()

	err = retry.IfNecessary(ctx, func() error {
		_, err := imageRef.NewImageSource(ctx, sysCtx)
		if err != nil {
			return err
		}
		return nil
	}, opts.RetryOpts)

	if err == nil {
		return true, nil
	} else if strings.Contains(err.Error(), "manifest unknown") {
		return false, nil
	} else {
		return false, err
	}
}

// delete - delete images
func (o *Mirror) delete(ctx context.Context, image string, opts *CopyOptions) error {

	if err := ReexecIfNecessaryForImages([]string{image}...); err != nil {
		return err
	}

	imageRef, err := alltransports.ParseImageName(image)
	if err != nil {
		return fmt.Errorf("invalid source name %s: %v", image, err)
	}

	sysCtx, err := opts.DestImage.NewSystemContext()
	if err != nil {
		return err
	}

	if strings.Contains(image, opts.LocalStorageFQDN) { // when copying to cache, use HTTP
		sysCtx.DockerInsecureSkipTLSVerify = types.OptionalBoolTrue
	}

	return retry.IfNecessary(ctx, func() error {
		err := imageRef.DeleteImage(ctx, sysCtx)
		if err != nil {
			return err
		}
		return nil
	}, opts.RetryOpts)
}

// determinePlatformSelection determines the image list selection and platform filters
// based on the provided copy options. It prioritizes InstancePlatforms (from ImageSetConfig)
// over the MultiArch flag.
func determinePlatformSelection(opts *CopyOptions) (copy.ImageListSelection, []copy.InstancePlatformFilter, error) {
	// Priority 1: Use InstancePlatforms if provided (set by batch worker from ImageSetConfig)
	if len(opts.InstancePlatforms) > 0 {
		platformsStr := strings.Join(opts.InstancePlatforms, ",")
		return parseMultiArch(platformsStr)
	}

	// Priority 2: Fall back to MultiArch flag (for CLI direct usage or default "all")
	if len(opts.MultiArch) > 0 && opts.All {
		return copy.CopySystemImage, nil, fmt.Errorf("MultiArch and All options cannot be used together")
	}

	imageListSelection := copy.CopySystemImage
	var instancePlatforms []copy.InstancePlatformFilter
	var err error

	if len(opts.MultiArch) > 0 {
		imageListSelection, instancePlatforms, err = parseMultiArch(opts.MultiArch)
		if err != nil {
			return copy.CopySystemImage, nil, err
		}
	}

	if opts.All {
		imageListSelection = copy.CopyAllImages
	}

	return imageListSelection, instancePlatforms, nil
}

// parseMultiArch parses the multi-arch option and returns the image list selection
// and optional platform filters for specific OS/Architecture combinations.
// Supports legacy options ('system', 'all', 'index-only') and new platform
// specifications (e.g., 'linux/amd64,linux/arm64').
func parseMultiArch(multiArch string) (copy.ImageListSelection, []copy.InstancePlatformFilter, error) {
	switch multiArch {
	case "system":
		return copy.CopySystemImage, nil, nil
	case "all":
		return copy.CopyAllImages, nil, nil
	// There is no CopyNoImages value in copy.ImageListSelection, but because we
	// don't provide an option to select a set of images to copy, we can use
	// CopySpecificImages.
	case "index-only":
		return copy.CopySpecificImages, nil, nil
	default:
		// Try to parse as comma-separated platform list (e.g., "linux/amd64,linux/arm64")
		// Parse comma-separated platform list
		var platforms []copy.InstancePlatformFilter
		for _, platform := range strings.Split(multiArch, ",") {
			platform = strings.TrimSpace(platform)
			parts := strings.Split(platform, "/")
			if len(parts) != 2 {
				return copy.CopySystemImage, nil, fmt.Errorf("unknown multi-arch option %q. Choose one of the supported options: 'system', 'all', 'index-only', or a comma-separated platform list like 'linux/amd64,linux/arm64'", multiArch)
			}
			platforms = append(platforms, copy.InstancePlatformFilter{
				OS:           parts[0],
				Architecture: parts[1],
			})
		}
		return copy.CopySpecificImages, platforms, nil
	}
}
