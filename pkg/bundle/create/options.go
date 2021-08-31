package create

import (
	"github.com/spf13/pflag"

	"github.com/RedHatGov/bundle/pkg/cli"
)

type Options struct {
	*cli.RootOptions

	OutputDir  string
	ConfigPath string
}

func (o *Options) BindFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&o.ConfigPath, "config", "c", "imageset-config.yaml", "Path to imageset configuration file")
	fs.StringVarP(&o.OutputDir, "output", "o", ".", "output directory for archives")
}
