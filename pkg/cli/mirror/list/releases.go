package list

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/cincinnati"
	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/image"
)

type ReleasesOptions struct {
	*cli.RootOptions
	Channel       string
	Channels      bool
	Version       string
	FilterOptions []string
}

// used to capture major.minor version from release tags
type releaseVersion struct {
	major int
	minor int
}

const OCPReleaseRepo = "quay.io/openshift-release-dev/ocp-release"

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
	fs.StringSliceVar(&o.FilterOptions, "filter-options", o.FilterOptions, "An architecture list to control the release image"+
		"picked when multiple variants are available")

	// TODO(jpower432): Make this flag visible again once release architecture selection
	// has been more thouroughly vetted
	if err := fs.MarkHidden("filter-options"); err != nil {
		logrus.Panic(err.Error())
	}

	o.BindFlags(cmd.PersistentFlags())

	return cmd
}

func (o *ReleasesOptions) Complete() error {
	if len(o.Version) > 0 && len(o.Channel) == 0 {

		r := releaseVersion{}
		if err := r.parseTag(o.Version); err != nil {
			return err
		}

		o.Channel = fmt.Sprintf("stable-%s", r.String())
	}
	if len(o.FilterOptions) == 0 {
		o.FilterOptions = []string{v1alpha2.DefaultPlatformArchitecture}
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
	for _, arch := range o.FilterOptions {
		if _, ok := cincinnati.SupportedArchs[arch]; !ok {
			return fmt.Errorf("architecture %q is not a supported release architecture", arch)
		}
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
		return listOCPReleaseVersions(w)
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

	for _, arch := range o.FilterOptions {
		vers, err := cincinnati.GetVersions(ctx, client, arch, o.Channel)
		if err != nil {
			return err
		}

		if _, err := fmt.Fprintf(w, "Channel: %v\nArchitecture: %v\n", o.Channel, arch); err != nil {
			return err
		}
		for _, ver := range vers {
			if _, err := fmt.Fprintf(w, "%s\n", ver); err != nil {
				return err
			}
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

func listOCPReleaseVersions(w io.Writer) error {
	tags, err := image.GetTagsFromImage(OCPReleaseRepo)
	if err != nil {
		return err
	}

	tagSlice := make([]string, 10)
	for tag, _ := range tags {
		tagSlice = append(tagSlice, tag)
	}
	versions := parseVersionTags(tagSlice)

	fmt.Fprint(w, "Available OpenShift Container Platform release versions: \n")

	for _, ver := range versions {
		fmt.Fprintf(w, "  %s\n", ver.String())
	}

	return nil
}

// Parse all the release image tags, and create a list of just the major.minor versions.
func parseVersionTags(versionTags []string) []releaseVersion {
	// Use a map to capture the unique major.minor versions
	ocpVersions := make(map[string]releaseVersion)
	for _, tag := range versionTags {
		r := releaseVersion{}
		if err := r.parseTag(tag); err != nil {
			// tag is not $major.$minor.$patch(.*) continue to next tag
			continue
		}
		ocpVersions[r.String()] = r
	}

	versions := make([]releaseVersion, 0, len(ocpVersions))
	for k := range ocpVersions {
		versions = append(versions, ocpVersions[k])
	}

	sort.Slice(versions, func(i, j int) bool {
		if versions[i].major == versions[j].major {
			return versions[i].minor < versions[j].minor
		}
		return versions[i].major < versions[j].major
	})
	return versions
}

func (r *releaseVersion) String() string {
	return fmt.Sprintf("%d.%d", r.major, r.minor)
}

func (r *releaseVersion) parseTag(tag string) error {
	s := strings.Split(tag, ".")
	if len(s) <= 1 {
		return errors.New("Unable parse major.minor version from: " + tag)
	}
	var err error
	r.major, err = strconv.Atoi(s[0])
	if err != nil {
		return errors.New("Unable to parse major version number. " + err.Error())
	}
	r.minor, _ = strconv.Atoi(s[1]) // if minor version unparsed, defaults to 0
	return nil
}
