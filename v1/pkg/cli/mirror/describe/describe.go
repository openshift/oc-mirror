package describe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/openshift/oc-mirror/pkg/bundle"
	"github.com/openshift/oc-mirror/pkg/cli"
)

type DescribeOptions struct {
	*cli.RootOptions
	From string
}

func NewDescribeCommand(f kcmdutil.Factory, ro *cli.RootOptions) *cobra.Command {
	o := DescribeOptions{}
	o.RootOptions = ro

	cmd := &cobra.Command{
		Use:   "describe <archive path>",
		Short: "Pretty print the contents of mirror metadata",
		Example: templates.Examples(`
			# Output the contents of 'mirror_seq1_00000.tar'
			oc-mirror describe mirror_seq1_00000.tar
		`),
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Complete(args))
			kcmdutil.CheckErr(o.Validate())
			kcmdutil.CheckErr(o.Run(cmd.Context()))
		},
	}

	o.BindFlags(cmd.PersistentFlags())

	return cmd
}

func (o *DescribeOptions) Complete(args []string) error {
	if len(args) == 1 {
		o.From = args[0]
	}
	return nil
}

func (o *DescribeOptions) Validate() error {
	if len(o.From) == 0 {
		return errors.New("must specify path to imageset archive")
	}
	return nil
}

func (o *DescribeOptions) Run(ctx context.Context) error {

	meta, err := bundle.ReadMetadataFromFile(ctx, o.From)
	if err != nil {
		return fmt.Errorf("error retrieving metadata from %q: %v", o.From, err)
	}

	// Process metadata for output
	data, err := json.MarshalIndent(&meta, "", " ")
	if err != nil {
		return err
	}
	fmt.Fprintln(o.IOStreams.Out, string(data))

	return nil
}
