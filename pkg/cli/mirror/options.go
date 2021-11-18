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
	DryRun           bool
	SourceSkipTLS    bool
	DestSkipTLS      bool
	SkipVerification bool
	SkipCleanup      bool
	SkipMissing      bool
	ContinueOnError  bool
	FilterOptions    imagemanifest.FilterOptions

	BuildxPlatforms []string
}

func (o *MirrorOptions) BindFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&o.ConfigPath, "config", "c", o.ConfigPath, "Path to imageset configuration file")
	fs.BoolVar(&o.SkipImagePin, "skip-image-pin", o.SkipImagePin, "Do not replace image tags with digest pins in operator catalogs")
	fs.StringVar(&o.From, "from", o.From, "The path to an input file (e.g. archived imageset)")
	fs.BoolVar(&o.ManifestsOnly, "manifests-only", o.ManifestsOnly, "Generate manifests and do not mirror")
	fs.BoolVar(&o.DryRun, "dry-run", o.DryRun, "Print actions without mirroring images "+
		"(experimental: only works for operator catalogs)")
	fs.BoolVar(&o.SourceSkipTLS, "source-skip-tls", o.SourceSkipTLS, "Skip client-side TLS validation for source")
	fs.BoolVar(&o.DestSkipTLS, "dest-skip-tls", o.DestSkipTLS, "Skip client-side TLS validation for destination")
	fs.BoolVar(&o.SkipVerification, "skip-verification", o.SkipVerification, "Skip digest verification")
	fs.BoolVar(&o.SkipCleanup, "skip-cleanup", o.SkipCleanup, "Skip removal of artifact directories")
	fs.StringVar(&o.FilterOptions.FilterByOS, "filter-by-os", "", "A regular expression to control which index image is picked when multiple variants are available")
	fs.StringSliceVar(&o.BuildxPlatforms, "buildx-platforms", o.BuildxPlatforms,
		"If set, the command will invoke 'docker buildx build' to build a catalog manifest list "+
			"for the specified platforms, ex. linux/amd64, instead of 'podman build' for the host platform. "+
			"The 'buildx' plugin and accompanying configuration MUST be installed on the build host. "+
			"This list does NOT filter operator bundle manifest list platforms within the catalog")
	fs.BoolVar(&o.ContinueOnError, "continue-on-error", o.ContinueOnError, "If an error occurs, keep going "+
		"and attempt to mirror as much as possible")
	fs.BoolVar(&o.SkipMissing, "skip-missing", o.SkipMissing, "If an input image is not found, skip them. "+
		"404/NotFound errors encountered while pulling images explicitly specified in the config "+
		"will not be skipped")
}
