package mirror

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"

	"github.com/openshift/oc-mirror/pkg/cli"
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
	SourcePlainHTTP  bool
	DestPlainHTTP    bool
	SkipVerification bool
	SkipCleanup      bool
	SkipMissing      bool
	ContinueOnError  bool
	FilterOptions    []string
	MaxPerRegistry   int
	// cancelCh is a channel listening for command cancellations
	cancelCh <-chan struct{}
	once     sync.Once
}

func (o *MirrorOptions) BindFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&o.ConfigPath, "config", "c", o.ConfigPath, "Path to imageset configuration file")
	fs.BoolVar(&o.SkipImagePin, "skip-image-pin", o.SkipImagePin, "Do not replace image tags with digest pins in operator catalogs")
	fs.StringVar(&o.From, "from", o.From, "The path to an input file (e.g. archived imageset)")
	fs.BoolVar(&o.ManifestsOnly, "manifests-only", o.ManifestsOnly, "Generate manifests and do not mirror")
	fs.BoolVar(&o.DryRun, "dry-run", o.DryRun, "Print actions without mirroring images "+
		"(experimental: only works for mirror to disk)")
	fs.BoolVar(&o.SourceSkipTLS, "source-skip-tls", o.SourceSkipTLS, "Disable TLS validation for source registry")
	fs.BoolVar(&o.DestSkipTLS, "dest-skip-tls", o.DestSkipTLS, "Disable TLS validation for destination registry")
	fs.BoolVar(&o.SourcePlainHTTP, "source-use-http", o.SourcePlainHTTP, "Use plain HTTP for source registry")
	fs.BoolVar(&o.DestPlainHTTP, "dest-use-http", o.DestPlainHTTP, "Use plain HTTP for destination registry")
	fs.BoolVar(&o.SkipVerification, "skip-verification", o.SkipVerification, "Skip digest verification")
	fs.BoolVar(&o.SkipCleanup, "skip-cleanup", o.SkipCleanup, "Skip removal of artifact directories")
	fs.StringSliceVar(&o.FilterOptions, "filter-by-os", o.FilterOptions, "A regular expression to control which release image is picked when multiple variants are available")
	fs.BoolVar(&o.ContinueOnError, "continue-on-error", o.ContinueOnError, "If an error occurs, keep going "+
		"and attempt to mirror as much as possible")
	fs.BoolVar(&o.SkipMissing, "skip-missing", o.SkipMissing, "If an input image is not found, skip them. "+
		"404/NotFound errors encountered while pulling images explicitly specified in the config "+
		"will not be skipped")
	fs.IntVar(&o.MaxPerRegistry, "max-per-registry", 2, "Number of concurrent requests allowed per registry")

	// TODO(jpower432): Make this flag visible again once release architecture selection
	// has been more thouroughly vetted
	if err := fs.MarkHidden("filter-by-os"); err != nil {
		logrus.Panic(err.Error())
	}
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
