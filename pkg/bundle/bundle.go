package bundle

import (
	"fmt"

	"github.com/spf13/cobra"

	"k8s.io/client-go/tools/clientcmd/api"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var (
	bundleExample = `
	# Reference a bundle-config.yaml to create a new full OCP bundle.
	%[1]s bundle create full --dir=bundle

	# Reference a bundle-config.yaml to create a new full OCP bundle.
	%[1]s bundle create diff --dir=bundle

	# Reference a bundle-config.yaml to create a new full OCP bundle.
	%[1]s bundle publish --from-bundle=bundle.x.y.z.tar.gz --to-directory=v2-directory --to-mirror=registry.url.local:5000 --install
`
)

// NamespaceOptions provides information required to update
// the current context on a user's KUBECONFIG
type NamespaceOptions struct {
	configFlags *genericclioptions.ConfigFlags

	resultingContext     *api.Context
	resultingContextName string

	userSpecifiedCluster   string
	userSpecifiedContext   string
	userSpecifiedAuthInfo  string
	userSpecifiedNamespace string

	rawConfig      api.Config
	listNamespaces bool
	args           []string

	genericclioptions.IOStreams
}

// NewNamespaceOptions provides an instance of NamespaceOptions with default values
func NewNamespaceOptions(streams genericclioptions.IOStreams) *NamespaceOptions {
	return &NamespaceOptions{
		configFlags: genericclioptions.NewConfigFlags(true),

		IOStreams: streams,
	}
}

// NewCmdNamespace provides a cobra command wrapping NamespaceOptions
func NewCmdNamespace(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewNamespaceOptions(streams)

	cmd := &cobra.Command{
		Use:          "bundle",
		Short:        "Manage OpenShift Container Image Bundles",
		Example:      fmt.Sprintf(bundleExample, "oc"),
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(c, args); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}
			if err := o.Run(); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&o.listNamespaces, "list", o.listNamespaces, "if true, print the list of all namespaces in the current KUBECONFIG")
	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}
