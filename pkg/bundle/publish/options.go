package publish

import (
	"os"

	"github.com/spf13/pflag"

	"github.com/RedHatGov/bundle/pkg/cli"
)

type Options struct {
	*cli.RootOptions

	ArchivePath string
	ToMirror    string

	tmp string
}

func (o *Options) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.ArchivePath, "archive", "", "The archive file path.")
	fs.StringVar(&o.ToMirror, "to-mirror", "", "The URL to the destination mirror registry")
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
