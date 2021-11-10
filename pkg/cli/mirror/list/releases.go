package list

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/RedHatGov/bundle/pkg/cincinnati"
	"github.com/RedHatGov/bundle/pkg/cli"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
)

type ReleasesOptions struct {
	*cli.RootOptions
	Channel  string
	Channels bool
	Version  string
}

func NewReleasesCommand(f kcmdutil.Factory, ro *cli.RootOptions) *cobra.Command {
	o := ReleasesOptions{}
	o.RootOptions = ro

	cmd := &cobra.Command{
		Use:   "releases",
		Short: "List available platform content and their version",
		Example: templates.Examples(`
			# Output all OCP release channels list for a release
            oc-mirror list releases --version=4.8
			# List all OCP versions in a specified channel
			oc-mirror list releases --channel=stable-4.8
			# List all OCP channels for a specific version
			oc-mirror list releases --channels --version=4.8
		`),
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Complete())
			kcmdutil.CheckErr(o.Validate())
			kcmdutil.CheckErr(o.Run(cmd.Context()))
		},
	}

	fs := cmd.Flags()
	fs.StringVar(&o.Channel, "channel", o.Channel, "List information for specified channel")
	fs.BoolVar(&o.Channels, "channels", o.Channels, "List all channel information")
	fs.StringVar(&o.Version, "version", o.Version, "Specify OpenShift release version")

	o.BindFlags(cmd.PersistentFlags())

	return cmd
}

func (o *ReleasesOptions) Complete() error {
	if len(o.Channel) == 0 {
		o.Channel = fmt.Sprintf("stable-%s", o.Version)
	}
	return nil
}

func (o *ReleasesOptions) Validate() error {
	if o.Channels && len(o.Version) == 0 {
		return errors.New("must specify --version")
	}
	if o.Channel == "stable-" {
		return errors.New("must specify --version or --channel")
	}
	return nil
}

func (o *ReleasesOptions) Run(ctx context.Context) error {

	w := o.IOStreams.Out

	c, url, err := cincinnati.NewClient(cincinnati.UpdateUrl, uuid.New())
	if err != nil {
		return err
	}

	if o.Channels {
		channels, err := c.GetChannels(ctx, url, o.Channel)
		if err != nil {
			return err
		}

		if _, err := fmt.Fprintf(w, "Listing channels for version %v.\n\n", o.Version); err != nil {
			return err
		}
		for channel := range channels {
			if _, err := fmt.Fprintf(w, "%s\n", channel); err != nil {
				return err
			}
		}
		return nil
	}

	// By default, the stable channel versions will be listed
	if strings.HasPrefix(o.Channel, "stable") {
		if _, err := fmt.Fprintln(w, "Listing stable channels. Use --channel=<channel-name> to filter."); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "User oc-mirror list release --channels to discover other channels."); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, ""); err != nil {
			return err
		}
	}

	vers, err := c.GetVersions(ctx, url, o.Channel)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, "Channel: %v\n", o.Channel); err != nil {
		return err
	}
	for _, ver := range vers {
		if _, err := fmt.Fprintf(w, "%s\n", ver); err != nil {
			return err
		}
	}

	return nil
}
