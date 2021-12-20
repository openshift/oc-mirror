package mirror

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/openshift/oc-mirror/pkg/cli"
	imagemanifest "github.com/openshift/oc/pkg/cli/image/manifest"
	"github.com/spf13/pflag"
)

type MirrorOptions struct {
	*cli.RootOptions
	OutputDir        string
	ConfigPath       string
	SkipImagePin     bool
	ManifestsOnly    bool
	From             string
	ToMirror         string
	UserNamespace    string
	DryRun           bool
	SourceSkipTLS    bool
	DestSkipTLS      bool
	SkipVerification bool
	SkipCleanup      bool
	SkipMissing      bool
	ContinueOnError  bool
	FilterOptions    imagemanifest.FilterOptions
	// cancelCh is a channel listening for command cancellations
	cancelCh    <-chan struct{}
	interrupted bool
}

func (o *MirrorOptions) BindFlags(fs *pflag.FlagSet) {
	o.init()
	fs.StringVarP(&o.ConfigPath, "config", "c", o.ConfigPath, "Path to imageset configuration file")
	fs.BoolVar(&o.SkipImagePin, "skip-image-pin", o.SkipImagePin, "Do not replace image tags with digest pins in operator catalogs")
	fs.StringVar(&o.From, "from", o.From, "The path to an input file (e.g. archived imageset)")
	fs.BoolVar(&o.ManifestsOnly, "manifests-only", o.ManifestsOnly, "Generate manifests and do not mirror")
	fs.BoolVar(&o.DryRun, "dry-run", o.DryRun, "Print actions without mirroring images "+
		"(experimental: only works for mirror to disk)")
	fs.BoolVar(&o.SourceSkipTLS, "source-skip-tls", o.SourceSkipTLS, "Use plain HTTP for source registry")
	fs.BoolVar(&o.DestSkipTLS, "dest-skip-tls", o.DestSkipTLS, "Use plain HTTP for destination registry")
	fs.BoolVar(&o.SkipVerification, "skip-verification", o.SkipVerification, "Skip digest verification")
	fs.BoolVar(&o.SkipCleanup, "skip-cleanup", o.SkipCleanup, "Skip removal of artifact directories")
	fs.StringVar(&o.FilterOptions.FilterByOS, "filter-by-os", "", "A regular expression to control which index image is picked when multiple variants are available")
	fs.BoolVar(&o.ContinueOnError, "continue-on-error", o.ContinueOnError, "If an error occurs, keep going "+
		"and attempt to mirror as much as possible")
	fs.BoolVar(&o.SkipMissing, "skip-missing", o.SkipMissing, "If an input image is not found, skip them. "+
		"404/NotFound errors encountered while pulling images explicitly specified in the config "+
		"will not be skipped")
}

func (o *MirrorOptions) init() {
	o.cancelCh = makeCancelCh(syscall.SIGINT, syscall.SIGTERM)
}

// CancelContext will return a cancellable context and listen for
// cancellation signals
func (o *MirrorOptions) CancelContext(parent context.Context) (context.Context, context.CancelFunc) {
	if o.cancelCh == nil {
		// return unchanged parent when cancel channel is
		// not initialized
		return parent, func() {}
	}
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
