package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/operator-framework/operator-registry/alpha/model"
	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/openshift/oc-mirror/v2/internal/pkg/cli/list"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/generate"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type generateOptions struct {
	outputType string
	outputDir  string
	prefix     string
	repos      []string
	globalOpts *mirror.CopyOptions
}

// NewGenerateCommand returns the `generate` command, which reverse-engineers
// ImageSetConfiguration and DeleteImageSetConfiguration files from a mirror registry.
func NewGenerateCommand(log clog.PluggableLoggerInterface, globalOpts *mirror.CopyOptions) *cobra.Command {
	o := &generateOptions{globalOpts: globalOpts}
	cmd := &cobra.Command{
		Use:   "generate docker://<registry>",
		Short: "Generate ImageSetConfiguration and DeleteImageSetConfiguration files from the content of a mirror registry",
		Example: templates.Examples(`
			# Generate both files in the current directory
			oc-mirror --v2 generate docker://registry.example.com:5000

			# Only a DeleteImageSetConfiguration, restricted to a registry path
			oc-mirror --v2 generate docker://registry.example.com:5000 --prefix=mirror/ --output-type=disc --output-dir=./out

			# Registry with /v2/_catalog disabled
			oc-mirror --v2 generate docker://registry.example.com:5000 --repos=openshift/release-images,redhat/redhat-operator-index
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			host, err := o.validate(args)
			if err != nil {
				return err
			}
			// NOTE: we don't want help output on errors from here onwards
			cmd.SilenceUsage = true
			return o.run(cmd.Context(), log, host)
		},
	}
	fs := cmd.Flags()
	fs.StringVar(&o.outputType, "output-type", string(generate.OutputBoth), "Files to generate: isc, disc or both")
	fs.StringVar(&o.outputDir, "output-dir", ".", "Directory the generated files are written to")
	fs.StringVar(&o.prefix, "prefix", "", "Only consider repositories starting with this registry path")
	fs.StringSliceVar(&o.repos, "repos", nil, "Repositories to inspect, for registries where /v2/_catalog is disabled")
	return cmd
}

func (o *generateOptions) validate(args []string) (string, error) {
	switch generate.OutputType(o.outputType) {
	case generate.OutputISC, generate.OutputDISC, generate.OutputBoth:
	default:
		return "", fmt.Errorf("invalid --output-type %q, must be isc, disc or both", o.outputType)
	}
	if len(args) < 1 {
		return "", errors.New("the registry argument is required, for example docker://registry.example.com:5000")
	}
	if !strings.HasPrefix(args[0], consts.DockerProtocol) {
		return "", fmt.Errorf("the registry argument must have a %s prefix", consts.DockerProtocol)
	}
	host := strings.TrimPrefix(args[0], consts.DockerProtocol)
	if strings.Contains(host, "/") {
		return "", errors.New("the registry argument must be host[:port] only, use --prefix to restrict to a registry path")
	}
	return host, nil
}

func (o *generateOptions) run(ctx context.Context, log clog.PluggableLoggerInterface, host string) error {
	sysCtx, err := o.globalOpts.SrcImage.NewSystemContext()
	if err != nil {
		return err
	}
	src := &generate.RegistrySource{
		Host:   host,
		Repos:  o.repos,
		SysCtx: sysCtx,
		LoadCatalog: func(ctx context.Context, ref string) (model.Model, error) {
			return list.DownloadCatalog(ctx, log, ref, *o.globalOpts)
		},
	}
	content, err := (&generate.Generator{Source: src, Host: host, Prefix: o.prefix}).Discover(ctx)
	if err != nil {
		return err
	}
	for _, w := range content.Warnings {
		log.Warn("%s", w)
	}
	paths, err := generate.WriteFiles(o.outputDir, content, generate.OutputType(o.outputType))
	if err != nil {
		return err
	}
	for _, p := range paths {
		log.Info("wrote %s", p)
	}
	return nil
}
