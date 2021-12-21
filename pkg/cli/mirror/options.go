package mirror

import (
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
}

func (o *MirrorOptions) BindFlags(fs *pflag.FlagSet) {
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
