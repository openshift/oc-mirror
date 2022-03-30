package list

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/openshift/oc-mirror/pkg/cincinnati"
	"github.com/openshift/oc-mirror/pkg/cli"
)

type ReleasesOptions struct {
	*cli.RootOptions
	Channel  string
	Channels bool
	Version  string
}

type ReleaseVersion struct {
	Major int
	Minor int
}

func NewReleasesCommand(f kcmdutil.Factory, ro *cli.RootOptions) *cobra.Command {
	o := ReleasesOptions{}
	o.RootOptions = ro

	cmd := &cobra.Command{
		Use:   "releases",
		Short: "List available platform content and versions",
		Example: templates.Examples(`
		    # Output OpenShift release versions
			oc-mirror list releases

			# Output all OpenShift release channels list for a release
			oc-mirror list releases --version=4.8

			# List all OpenShift versions in a specified channel
			oc-mirror list releases --channel=stable-4.8

			# List all OpenShift channels for a specific version
			oc-mirror list releases --channels --version=4.8
		`),
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Complete())
			kcmdutil.CheckErr(o.Validate())
			kcmdutil.CheckErr(o.Run(cmd.Context()))
		},
	}

	fs := cmd.Flags()
	fs.StringVar(&o.Channel, "channel", o.Channel, "List information for a specified channel")
	fs.BoolVar(&o.Channels, "channels", o.Channels, "List all channel information")
	fs.StringVar(&o.Version, "version", o.Version, "Specify an OpenShift release version")

	o.BindFlags(cmd.PersistentFlags())

	return cmd
}

func (o *ReleasesOptions) Complete() error {
	if len(o.Version) > 0 && len(o.Channel) == 0 {
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

	client, err := cincinnati.NewOCPClient(uuid.New())
	if err != nil {
		return err
	}

	if o.Channels {
		return listChannelsForVersion(ctx, client, o, w)
	}

	if len(o.Channel) == 0 {
		return listOCPReleaseVersions()
	}

	return listChannels(o, w, ctx, client)

}

func listChannels(o *ReleasesOptions, w io.Writer, ctx context.Context, client cincinnati.Client) error {
	// By default, the stable channel versions will be listed
	if strings.HasPrefix(o.Channel, "stable") {
		if _, err := fmt.Fprintln(w, "Listing stable channels. Use --channel=<channel-name> to filter."); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "Use oc-mirror list release --channels to discover other channels."); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, ""); err != nil {
			return err
		}
	}

	vers, err := cincinnati.GetVersions(ctx, client, o.Channel)
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

func listChannelsForVersion(ctx context.Context, client cincinnati.Client, o *ReleasesOptions, w io.Writer) error {
	channels, err := cincinnati.GetChannels(ctx, client, o.Channel)
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

func listOCPReleaseVersions() error {

	repo, err := name.NewRepository("quay.io/openshift-release-dev/ocp-release")
	if err != nil {
		return err
	}
	versionTags, err := remote.List(repo, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return err
	}

	ocpVersions := make(map[string]ReleaseVersion)
	for _, tag := range versionTags {
		s := strings.Split(tag, ".")
		if len(s) > 1 {
			var r ReleaseVersion
			r.Major, err = strconv.Atoi(s[0])
			if err != nil {
				continue // tag is not $major.$minor.$patch(.*) continue to next tag
			}
			r.Minor, _ = strconv.Atoi(s[1])
			v := fmt.Sprintf("%d.%d", r.Major, r.Minor)
			ocpVersions[v] = r
		}

	}

	versions := make([]ReleaseVersion, 0, len(ocpVersions))
	for k := range ocpVersions {
		versions = append(versions, ocpVersions[k])
	}

	sort.Slice(versions, func(i, j int) bool {
		if versions[i].Major == versions[j].Major {
			return versions[i].Minor < versions[j].Minor
		}
		return versions[i].Major < versions[j].Major
	})

	logrus.Info("OCP Versions: ", versions)

	return nil
}
