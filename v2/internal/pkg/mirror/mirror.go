package mirror

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/containers/common/pkg/retry"
	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/pkg/cli"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/distribution/reference"

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

	imageListSelection := copy.CopySystemImage
	if len(opts.MultiArch) > 0 && opts.All {
		return fmt.Errorf("cannot use --all and --multi-arch flags together")
	}

	if len(opts.MultiArch) > 0 {
		imageListSelection, err = parseMultiArch(opts.MultiArch)
		if err != nil {
			return err
		}
	}

	if opts.All {
		imageListSelection = copy.CopyAllImages
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
		// Retry on 502, 503, and 504 server errors, they appear to be quite common in the field
		// We duplicate this here because older versions of oc-mirror cannot bump containers/common given Golang version restrictions
		if httpError.StatusCode >= http.StatusBadGateway && httpError.StatusCode <= http.StatusGatewayTimeout {
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

// parseMultiArch
func parseMultiArch(multiArch string) (copy.ImageListSelection, error) {
	switch multiArch {
	case "system":
		return copy.CopySystemImage, nil
	case "all":
		return copy.CopyAllImages, nil
	// There is no CopyNoImages value in copy.ImageListSelection, but because we
	// don't provide an option to select a set of images to copy, we can use
	// CopySpecificImages.
	case "index-only":
		return copy.CopySpecificImages, nil
	// We don't expose CopySpecificImages other than index-only above, because
	// we currently don't provide an option to choose the images to copy. That
	// could be added in the future.
	default:
		return copy.CopySystemImage, fmt.Errorf("unknown multi-arch option %q. Choose one of the supported options: 'system', 'all', or 'index-only'", multiArch)
	}
}
