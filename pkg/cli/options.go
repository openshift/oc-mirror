package cli

import (
	"flag"
	"io"
	"io/ioutil"
	"log"
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
	//TODO tell int is needed vs str now
	fs.IntVarP(&o.LogLevel, "verbose", "v", 2, "Log level (e.g. \"Error (1), Info (2) | Warning (3) | Debug (4)\")")
	if err := fs.MarkHidden("dir"); err != nil {
		klog.Fatal(err.Error())
	}
}

func (o *RootOptions) LogfilePreRun(cmd *cobra.Command, _ []string) {

	var fsv2 flag.FlagSet
	klog.InitFlags(&fsv2)
	checkErr(fsv2.Set("stderrthreshold", "4"))

	f, err := os.OpenFile("oc-mirror.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()
	wrt := io.MultiWriter(ioutil.Discard, f)

	klog.SetOutput(wrt)
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
