package cli

import (
	"flag"
	"fmt"
	"io"
	"os"

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
	fs.IntVarP(&o.LogLevel, "verbose", "v", 2, "Log level (e.g. \"Error (1), Info (2) | Warning (3) | Debug (4)\")")
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

	o.logfileCleanup = func() {
		klog.Flush()
		checkErr(logFile.Close())
	}

	klog.SetOutput(io.MultiWriter(o.IOStreams.Out, logFile))
	// Add to root IOStream options
	o.IOStreams = genericclioptions.IOStreams{
		In:     o.IOStreams.In,
		Out:    io.MultiWriter(o.IOStreams.Out, logFile),
		ErrOut: io.MultiWriter(o.IOStreams.ErrOut, logFile),
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
