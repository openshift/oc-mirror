// Package list implements the `list` command
package list

import (
	"github.com/spf13/cobra"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func NewListCommand(log clog.PluggableLoggerInterface, opts *mirror.CopyOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available platform and operator content and their versions",
	}

	cmd.AddCommand(NewListOperatorsCommand(log, opts))
	cmd.AddCommand(NewListReleasesCommand(log, opts))

	return cmd
}
