package publish

import (
	"os"

	"github.com/spf13/pflag"

	"github.com/RedHatGov/bundle/pkg/cli"
)

type Options struct {
	*cli.RootOptions

	ArchivePath     string
	ToMirror        string
	OutputDir       string
	BuildxPlatforms []string
}

func (o *Options) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.ArchivePath, "archive", "", "The archive file path.")
	fs.StringVar(&o.ToMirror, "to-mirror", "", "The URL to the destination mirror registry")
	fs.StringSliceVar(&o.BuildxPlatforms, "buildx-platforms", nil,
		"If set, the command will invoke 'docker buildx build' to build a catalog manifest list "+
			"for the specified platforms, ex. linux/amd64, instead of 'podman build' for the host platform. "+
			"The 'buildx' plugin and accompanying configuration MUST be installed on the build host. "+
			"This list does NOT filter operator bundle manifest list platforms within the catalog")
	fs.StringVar(&o.OutputDir, "output", "", "Output directory for publish result artifacts")
}

// ValidatePaths validate the existence of paths from user flags
func (o *Options) ValidatePaths() error {
	if _, err := os.Stat(o.ArchivePath); err != nil {
		return err
	}
	if _, err := os.Stat(o.Dir); err != nil {
		return err
	}

	return nil
}
