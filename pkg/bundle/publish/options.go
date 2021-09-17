package publish

import (
	"os"

	"github.com/spf13/pflag"

	"github.com/RedHatGov/bundle/pkg/cli"
)

type Options struct {
	*cli.RootOptions

	ArchivePath      string
	ToMirror         string
	CatalogPlatforms []string
}

var defaultPlatforms = []string{"linux/amd64"}

func (o *Options) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.ArchivePath, "archive", "", "The archive file path.")
	fs.StringVar(&o.ToMirror, "to-mirror", "", "The URL to the destination mirror registry")
	fs.StringSliceVar(&o.CatalogPlatforms, "catalog-platforms", defaultPlatforms, "Platforms to build a catalog manifest list for. "+
		"This list does NOT filter operator bundle manifest list platforms within the catalog")
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
