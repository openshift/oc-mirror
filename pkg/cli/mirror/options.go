package mirror

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/spf13/pflag"

	"github.com/openshift/oc-mirror/pkg/cli"
)

type MirrorOptions struct {
	*cli.RootOptions
	OutputDir                  string // directory path, whose value is dependent on how oc mirror was invoked
	ConfigPath                 string // Path to imageset configuration file
	SkipImagePin               bool   // Do not replace image tags with digest pins in operator catalogs
	ManifestsOnly              bool   // Generate manifests and do not mirror
	From                       string // Path to an input file (e.g. archived imageset)
	ToMirror                   string // Final destination for the mirror operation
	UserNamespace              string // The <namespace>/<image> portion of a docker reference only
	DryRun                     bool   // Print actions without mirroring images
	SourceSkipTLS              bool   // Disable TLS validation for source registry
	DestSkipTLS                bool   // Disable TLS validation for destination registry
	V2                         bool   // Redirect the flow to oc-mirror v2 - PLEASE DO NOT USE that. V2 is still under development and it is not ready to be used.
	V1                         bool   // Redirect the flow to oc-mirror v1 - This flag is going to redirect the flow to v1 (legacy code) when v2 becomes the default (still under development).
	SourcePlainHTTP            bool   // Use plain HTTP for source registry
	DestPlainHTTP              bool   // Use plain HTTP for destination registry
	SkipVerification           bool   // Skip verifying the integrity of the retrieved content.
	SkipCleanup                bool   // Skip removal of artifact directories
	SkipMissing                bool   // If an input image is not found, skip them.
	SkipMetadataCheck          bool   // Skip metadata when publishing an imageset
	SkipPruning                bool   // If set, will disable pruning globally
	ContinueOnError            bool   // If an error occurs, keep going and attempt to complete operations if possible
	IgnoreHistory              bool   // Ignore past mirrors when downloading images and packing layers
	MaxPerRegistry             int    // Number of concurrent requests allowed per registry
	OCIRegistriesConfig        string // Registries config file location (it works only with local oci catalogs)
	OCIInsecureSignaturePolicy bool   // If set, OCI catalog push will not try to push signatures
	MaxNestedPaths             int
	// cancelCh is a channel listening for command cancellations
	cancelCh                          <-chan struct{}
	once                              sync.Once
	continuedOnError                  bool
	remoteRegFuncs                    RemoteRegFuncs
	operatorCatalogToFullArtifactPath map[string]string // stores temporary paths to declarative config directory key: OCI URI (e.g. oci://foo which originates with v1alpha2.Operator.Catalog) value: <current working directory>/olm_artifacts/<repo>/<config folder>
}

func (o *MirrorOptions) BindFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&o.ConfigPath, "config", "c", o.ConfigPath, "Path to imageset configuration file")
	fs.BoolVar(&o.SkipImagePin, "skip-image-pin", o.SkipImagePin, "Do not replace image tags with digest pins in operator catalogs")
	fs.StringVar(&o.From, "from", o.From, "Path to an input file (e.g. archived imageset)")
	fs.BoolVar(&o.ManifestsOnly, "manifests-only", o.ManifestsOnly, "Generate manifests and do not mirror")
	fs.BoolVar(&o.DryRun, "dry-run", o.DryRun, "Print actions without mirroring images")
	fs.BoolVar(&o.SourceSkipTLS, "source-skip-tls", o.SourceSkipTLS, "Disable TLS validation for source registry")
	fs.BoolVar(&o.DestSkipTLS, "dest-skip-tls", o.DestSkipTLS, "Disable TLS validation for destination registry")
	fs.BoolVar(&o.V2, "v2", o.V2, "Redirect the flow to oc-mirror v2 - This is Tech Preview, it is still under development and it is not production ready.")
	fs.BoolVar(&o.V1, "v1", o.V1, "Redirect the flow to oc-mirror v1 - This flag is going to redirect the flow to v1 (legacy code) when v2 becomes the default (still under development).")
	fs.BoolVar(&o.SourcePlainHTTP, "source-use-http", o.SourcePlainHTTP, "Use plain HTTP for source registry")
	fs.BoolVar(&o.DestPlainHTTP, "dest-use-http", o.DestPlainHTTP, "Use plain HTTP for destination registry")
	fs.BoolVar(&o.SkipVerification, "skip-verification", o.SkipVerification, "Skip verifying the integrity of the retrieved content."+
		"This is not recommended, but may be necessary when importing images from older image registries."+
		"Only bypass verification if the registry is known to be trustworthy.")
	fs.BoolVar(&o.SkipCleanup, "skip-cleanup", o.SkipCleanup, "Skip removal of artifact directories")
	fs.BoolVar(&o.IgnoreHistory, "ignore-history", o.IgnoreHistory, "Ignore past mirrors when downloading images and packing layers")
	fs.BoolVar(&o.SkipMetadataCheck, "skip-metadata-check", o.SkipMetadataCheck, "Skip metadata when publishing an imageset."+
		"This is only recommended when the imageset was created --ignore-history")
	fs.BoolVar(&o.ContinueOnError, "continue-on-error", o.ContinueOnError, "If an error occurs, keep going "+
		"and attempt to complete operations if possible")
	fs.BoolVar(&o.SkipMissing, "skip-missing", o.SkipMissing, "If an input image is not found, skip them. "+
		"404/NotFound errors encountered while pulling images explicitly specified in the config "+
		"will not be skipped")
	fs.IntVar(&o.MaxPerRegistry, "max-per-registry", 6, "Number of concurrent requests allowed per registry")
	fs.StringVar(&o.OCIRegistriesConfig, "oci-registries-config", o.OCIRegistriesConfig, "Registries config file location (it works only with local oci catalogs)")
	fs.BoolVar(&o.OCIInsecureSignaturePolicy, "oci-insecure-signature-policy", o.OCIInsecureSignaturePolicy, "If set, OCI catalog push will not try to push signatures")
	fs.BoolVar(&o.SkipPruning, "skip-pruning", o.SkipPruning, "If set, will disable pruning globally")
	fs.IntVar(&o.MaxNestedPaths, "max-nested-paths", 0, "Number of nested paths, for destination registries that limit nested paths")
}

func (o *MirrorOptions) init() {
	o.cancelCh = makeCancelCh(syscall.SIGINT, syscall.SIGTERM)
}

// CancelContext will return a cancellable context and listen for
// cancellation signals
func (o *MirrorOptions) CancelContext(parent context.Context) (context.Context, context.CancelFunc) {
	o.once.Do(o.init)
	ctx, cancel := context.WithCancel(parent)
	go func() {
		select {
		case <-o.cancelCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

// makeCancelCh creates an interrupt listener for os signals
// and will send a message on a returned channel
func makeCancelCh(signals ...os.Signal) <-chan struct{} {
	resultCh := make(chan struct{})
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, signals...)
	go func() {
		for {
			<-signalCh
			resultCh <- struct{}{}
		}
	}()
	return resultCh
}
