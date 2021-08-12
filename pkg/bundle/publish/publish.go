package bundle

import (
	"os"

	"github.com/openshift/oc/pkg/cli/image/mirror"
	"github.com/sirupsen/logrus"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var (
	PublishOpts struct {
		FromBundle string
		ToMirror   string
	}
)

// load incoming tar

// check sequence

// import containers

// import imagecontentsourcepolicy

// import catalogsource

// import metadata

// mirror to registry
func mirrorToReg() {

	in, out, errout := os.Stdin, os.Stdout, os.Stderr

	iostreams := genericclioptions.IOStreams{In: in, Out: out, ErrOut: errout}

	imageOpts := mirror.NewMirrorImageOptions(iostreams)
	// oc image mirror -a dba-ps.json --from-dir=release/ "file://openshift/release:4.7.3*" registry.dbasparta.io:5020/openshift --insecure
	imageOpts.FromFileDir = PublishOpts.FromBundle
	//	imageOpts.Out = PublishOpts.ToMirror
	// imageOpts.Filenames = "file://openshift/release:4.7.3*"
	logrus.Info("Dry Run: ", imageOpts.DryRun)
	logrus.Info("From File Dir: ", imageOpts.FromFileDir)

}

// install imagecontentsourcepolicy

// install catalogsource

func Publish(rootDir string) error {
	logrus.Infoln("Publish bundle package")

	logrus.Info("From Bundle: ", PublishOpts.FromBundle)
	logrus.Info("Mirror: ", PublishOpts.ToMirror)

	mirrorToReg()

	return nil
}
