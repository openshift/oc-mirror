package cli

import (
	"io/ioutil"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/crypto/ssh/terminal"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type RootOptions struct {
	genericclioptions.IOStreams

	Dir         string
	LogLevel    string
	DryRun      bool
	SkipTLS     bool
	SkipCleanup bool

	logfileCleanup func()
}

func (o *RootOptions) BindFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&o.Dir, "dir", "d", ".", "assets directory")
	fs.StringVar(&o.LogLevel, "log-level", "info", "log level (e.g. \"debug | info | warn | error\")")
	fs.BoolVar(&o.DryRun, "dry-run", false, "print actions without mirroring images "+
		"(experimental: only works for operator catalogs)")
	fs.BoolVar(&o.SkipTLS, "skip-tls", false, "skip client-side TLS validation")
	fs.BoolVar(&o.SkipCleanup, "skip-cleanup", false, "skip removal of artifact directories")
}

func (o *RootOptions) LogfilePreRun(cmd *cobra.Command, _ []string) {
	logrus.SetOutput(ioutil.Discard)
	logrus.SetLevel(logrus.TraceLevel)

	level, err := logrus.ParseLevel(o.LogLevel)
	if err != nil {
		logrus.Fatalf("parse root options log-level: %v", err)
	}

	logrus.AddHook(newFileHookWithNewlineTruncate(os.Stderr, level, &logrus.TextFormatter{
		// Setting ForceColors is necessary because logrus.TextFormatter determines
		// whether or not to enable colors by looking at the output of the logger.
		// In this case, the output is ioutil.Discard, which is not a terminal.
		// Overriding it here allows the same check to be done, but against the
		// hook's output instead of the logger's output.
		ForceColors:            terminal.IsTerminal(int(os.Stderr.Fd())),
		DisableTimestamp:       true,
		DisableLevelTruncation: true,
		DisableQuote:           true,
	}))

	o.logfileCleanup = setupFileHook(o.Dir)
}

func (o *RootOptions) LogfilePostRun(*cobra.Command, []string) {
	if o.logfileCleanup != nil {
		o.logfileCleanup()
	}
}
