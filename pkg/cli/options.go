package cli

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
)

type RootOptions struct {
	genericclioptions.IOStreams

	Dir      string
	LogLevel int

	logfileCleanup func()
}

func (o *RootOptions) BindFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&o.Dir, "dir", "d", "oc-mirror-workspace", "Assets directory")
	fs.IntVarP(&o.LogLevel, "verbose", "v", o.LogLevel, "Number for the log level verbosity (valid 1-9, default is 0)")
	if err := fs.MarkHidden("dir"); err != nil {
		klog.Fatal(err.Error())
	}
}

func (o *RootOptions) LogfilePreRun(cmd *cobra.Command, _ []string) {
	var fsv2 flag.FlagSet
	// Configure klog flags
	klog.InitFlags(&fsv2)
	checkErr(fsv2.Set("stderrthreshold", "4"))
	checkErr(fsv2.Set("skip_headers", "true"))
	checkErr(fsv2.Set("logtostderr", "false"))
	checkErr(fsv2.Set("alsologtostderr", "false"))
	checkErr(fsv2.Set("v", fmt.Sprintf("%d", o.LogLevel)))

	logFile, err := os.OpenFile(".oc-mirror.log", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0666)
	if err != nil {
		klog.Fatal(err)
	}

	klog.SetOutput(io.MultiWriter(o.IOStreams.Out, logFile))

	// Setup logrus for use with operator-registry
	logrus.SetOutput(ioutil.Discard)

	var logrusLevel logrus.Level
	switch o.LogLevel {
	case 0:
		logrusLevel = logrus.InfoLevel
	case 1:
		logrusLevel = logrus.DebugLevel
	case 2:
		logrusLevel = logrus.DebugLevel
	default:
		logrusLevel = logrus.TraceLevel
	}

	logrus.SetLevel(logrusLevel)
	logrus.AddHook(newFileHookWithNewlineTruncate(o.IOStreams.ErrOut, logrusLevel, &logrus.TextFormatter{
		DisableTimestamp:       true,
		DisableLevelTruncation: true,
		DisableQuote:           true,
	}))
	logrusCleanup := setupFileHook(logFile)

	// Add to root IOStream options
	o.IOStreams = genericclioptions.IOStreams{
		In:     o.IOStreams.In,
		Out:    io.MultiWriter(o.IOStreams.Out, logFile),
		ErrOut: io.MultiWriter(o.IOStreams.ErrOut, logFile),
	}

	o.logfileCleanup = func() {
		klog.Flush()
		logrusCleanup()
		checkErr(logFile.Close())
	}

}

func (o *RootOptions) LogfilePostRun(*cobra.Command, []string) {
	if o.logfileCleanup != nil {
		o.logfileCleanup()
	}
}

func checkErr(err error) {
	if err != nil {
		klog.Fatal(err)
	}
}
