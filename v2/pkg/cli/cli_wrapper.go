package cli

import (
	"fmt"

	"github.com/openshift/oc-mirror/v2/internal/pkg/cli"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/spf13/cobra"
)

func V2Cmd(loglevel string) *cobra.Command {
	log := clog.New(loglevel)

	fmt.Println()
	log.Warn("⚠️  --v2 flag identified, flow redirected to the oc-mirror v2 version. This is Tech Preview, it is still under development and it is not production ready.")

	cmd := cli.NewMirrorCmd(log)
	return cmd
}
