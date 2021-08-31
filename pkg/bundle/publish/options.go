package publish

import (
	"github.com/spf13/pflag"

	"github.com/RedHatGov/bundle/pkg/cli"
)

type Options struct {
	*cli.RootOptions

	ArchivePath string
	ToMirror    string
}

func (o *Options) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.ArchivePath, "archive", "", "The archive file path.")
	fs.StringVar(&o.ToMirror, "to-mirror", "", "The URL to the destination mirror registry")
}
