package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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

// generateOptions holds the flag values of the generate command.
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
		Args: cobra.ExactArgs(1),
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

// validate checks the flags and arguments and returns the registry host
// taken from the single docker://<registry> argument.
func (o *generateOptions) validate(args []string) (string, error) {
	switch generate.OutputType(o.outputType) {
	case generate.OutputISC, generate.OutputDISC, generate.OutputBoth:
	default:
		return "", errors.New("invalid --output-type, must be isc, disc or both")
	}
	if len(args) < 1 {
		return "", errors.New("the registry argument is required, for example docker://registry.example.com:5000")
	}
	if !strings.HasPrefix(args[0], consts.DockerProtocol) {
		return "", fmt.Errorf("the registry argument must have a %s prefix", consts.DockerProtocol)
	}
	host := strings.TrimPrefix(args[0], consts.DockerProtocol)
	if host == "" {
		return "", errors.New("the registry argument has an empty host, for example docker://registry.example.com:5000")
	}
	if strings.Contains(host, "/") {
		return "", errors.New("the registry argument must be host[:port] only, use --prefix to restrict to a registry path")
	}
	return host, nil
}

// run discovers the content of the registry at host and writes the requested
// config files into the output directory.
func (o *generateOptions) run(ctx context.Context, log clog.PluggableLoggerInterface, host string) error {
	sysCtx, err := o.globalOpts.SrcImage.NewSystemContext()
	if err != nil {
		return errors.New("could not configure registry access; check source authentication and TLS options")
	}
	src := &generate.RegistrySource{
		Host:   host,
		Repos:  o.repos,
		SysCtx: sysCtx,
		LoadCatalog: func(ctx context.Context, ref string) (model.Model, error) {
			opts := *o.globalOpts
			opts.Stdout = io.Discard
			// The copy layer also creates a logger from Global.LogLevel.
			global := *opts.Global
			global.LogLevel = "info"
			opts.Global = &global
			return list.DownloadCatalog(ctx, generateCatalogLogger{log}, ref, opts)
		},
	}
	content, err := (&generate.Generator{Source: src, Host: host, Prefix: o.prefix}).Discover(ctx)
	if err != nil {
		return errors.New("could not discover registry content; check registry access, catalog availability, and --repos")
	}
	// Warnings embed repository names and registry error text, so they go to the
	// generated files only, not to the logs.
	if n := len(content.Warnings); n > 0 {
		log.Warn("%d warnings recorded, see the NOTE comments at the top of the generated files", n)
	}
	paths, err := generate.WriteFiles(o.outputDir, content, generate.OutputType(o.outputType))
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("generated configuration already exists; use another --output-dir: %w", os.ErrExist)
		}
		return errors.New("could not write generated configuration; check --output-dir exists and is writable")
	}
	log.Info("wrote %d configuration files to --output-dir", len(paths))
	return nil
}

// generateCatalogLogger suppresses catalog diagnostics containing registry identifiers.
// The command reports discovery failures and warning counts separately.
type generateCatalogLogger struct{ clog.PluggableLoggerInterface }

// Error suppresses raw catalog errors.
func (generateCatalogLogger) Error(string, ...interface{}) {}

// Warn suppresses catalog warnings containing bundle identifiers.
func (generateCatalogLogger) Warn(string, ...interface{}) {}

// Info suppresses catalog progress containing image references.
func (generateCatalogLogger) Info(string, ...interface{}) {}

// Debug suppresses catalog paths and image references.
func (generateCatalogLogger) Debug(string, ...interface{}) {}

// Trace suppresses detailed catalog diagnostics.
func (generateCatalogLogger) Trace(string, ...interface{}) {}
