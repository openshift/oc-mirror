package list

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"go.podman.io/image/v5/docker"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/cincinnati"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

const ocpReleaseRepo = "quay.io/openshift-release-dev/ocp-release"

type listReleasesOptions struct {
	channels    bool
	channel     string
	version     string
	filterArchs []string
	copyOpts    *mirror.CopyOptions
}

// NewListReleasesCommand returns a `list releases` command
func NewListReleasesCommand(log clog.PluggableLoggerInterface, copyOpts *mirror.CopyOptions) *cobra.Command {
	opts := listReleasesOptions{copyOpts: copyOpts}
	cmd := &cobra.Command{
		Use:   "releases",
		Short: "List available platform content and versions",
		Example: templates.Examples(`
			# Output OpenShift release versions
			oc-mirror --v2 list releases

			# Output all OpenShift releases for a specific version
			oc-mirror --v2 list releases --version=4.8

			# List all OpenShift releases in a specified channel
			oc-mirror --v2 list releases --channel=stable-4.8

			# List OpenShift versions for a specific release and one or more architectures. Valid architectures: amd64 (default), arm64, ppc64le, s390x, multi.
			oc-mirror --v2 list releases --channel=fast-4.13 --version=4.13 --filter-by-archs amd64,arm64,ppc64le,s390x,multi

			# List all OpenShift channels for a specific version
			oc-mirror --v2 list releases --channels --version=4.8

			# List OpenShift channels for a specific version.
			oc-mirror --v2 list releases --channels --version=4.13
		`),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.channels && len(opts.version) == 0 {
				return errors.New("must specify --version with --channels")
			}
			if len(opts.version) > 0 && len(opts.channel) == 0 {
				opts.channel = fmt.Sprintf("stable-%s", opts.version)
			}
			if opts.channel == "stable-" {
				return errors.New("must specify --version or --channel")
			}
			if len(opts.filterArchs) == 0 {
				opts.filterArchs = append(opts.filterArchs, v2alpha1.DefaultPlatformArchitecture)
			}
			validArches := sets.New("amd64", "arm64", "s390x", "ppc64le", "multi")
			if diff := sets.New(opts.filterArchs...).Difference(validArches); diff.Len() > 0 {
				return fmt.Errorf("invalid architecture(s) %v. Known architectures: %v", diff.UnsortedList(), sets.List(validArches))
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// NOTE: we don't want help output on errors from here onwards
			cmd.SilenceUsage = true
			return runListReleases(cmd.Context(), log, &opts)
		},
	}

	fs := cmd.Flags()
	fs.StringVar(&opts.version, "version", "", "Specify an Openshift release version.")
	fs.BoolVar(&opts.channels, "channels", false, "List all channel information. Requires --version.")
	fs.StringVar(&opts.channel, "channel", "", "List information for a specific channel. Defaults to the stable channel.")
	fs.StringSliceVar(&opts.filterArchs, "filter-by-archs", []string{v2alpha1.DefaultPlatformArchitecture}, "Architecture list to control the release image picked when multiple variants are available.")

	return cmd
}

func runListReleases(ctx context.Context, log clog.PluggableLoggerInterface, opts *listReleasesOptions) error {
	w := os.Stdout

	updateURL := cincinnati.OcpUpdateURL
	if override := os.Getenv("UPDATE_URL_OVERRIDE"); override != "" {
		updateURL = override
	}

	if opts.channels {
		return listChannelsForVersion(ctx, log, w, updateURL, opts)
	}

	if len(opts.channel) == 0 {
		return listReleaseMajorVersions(ctx, w, ocpReleaseRepo, opts)
	}

	return listVersionsForChannel(ctx, log, w, updateURL, opts)
}

func listChannelsForVersion(ctx context.Context, log clog.PluggableLoggerInterface, w io.Writer, releaseURL string, opts *listReleasesOptions) error {
	// Channels are the same for all arches, so we don't need to set `arch` to any specific value
	graph, err := loadGraphData(ctx, log, releaseURL, "", opts.channel)
	if err != nil {
		return fmt.Errorf("channel %q: %w", opts.channel, err)
	}
	channels := graph.GetChannels()
	fmt.Fprintf(w, "Listing channels for version %v:\n\n", opts.version)
	for _, ch := range sets.List(channels) {
		fmt.Fprintf(w, "%s\n", ch)
	}

	return nil
}

func listReleaseMajorVersions(ctx context.Context, w io.Writer, releaseImage string, opts *listReleasesOptions) error {
	versions, err := getXYVersionsFromImage(ctx, releaseImage, opts)
	if err != nil {
		return err
	}

	sortedVers := versions.UnsortedList()
	slices.SortFunc(sortedVers,
		func(r1 releaseVer, r2 releaseVer) int {
			if v := cmp.Compare(r1.major, r2.major); v == 0 {
				return cmp.Compare(r1.minor, r2.minor)
			} else {
				return v
			}
		},
	)

	fmt.Fprintf(w, "Available OpenShift Container Platform release versions:\n")
	for _, ver := range sortedVers {
		fmt.Fprintf(w, "  %s\n", ver)
	}

	return nil
}

type releaseVer struct {
	major int
	minor int
}

func (r releaseVer) String() string {
	return fmt.Sprintf("%d.%d", r.major, r.minor)
}

// Go through the container image tags and extract the X.Y versions from them.
func getXYVersionsFromImage(ctx context.Context, image string, opts *listReleasesOptions) (sets.Set[releaseVer], error) {
	ref, err := docker.ParseReference("//" + image)
	if err != nil {
		return nil, fmt.Errorf("failed to parse release reference: %w", err)
	}

	sysCtx, err := opts.copyOpts.SrcImage.NewSystemContext()
	if err != nil {
		return nil, err
	}

	tags, err := docker.GetRepositoryTags(ctx, sysCtx, ref)
	if err != nil {
		return nil, fmt.Errorf("failed get release tags: %w", err)
	}

	// Tag format (roughly): X.Y.Z-Patch-Arch
	versionRegex := regexp.MustCompile(`(\d+)\.(\d+).*`)
	versions := sets.New[releaseVer]()
	for _, vt := range tags {
		// Skip signature tags
		if strings.HasSuffix(vt, ".sig") {
			continue
		}
		matches := versionRegex.FindStringSubmatch(vt)
		if len(matches) < 3 {
			continue
		}
		major, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, fmt.Errorf("unable to parse major version from tag %q", vt)
		}
		minor, err := strconv.Atoi(matches[2])
		if err != nil {
			return nil, fmt.Errorf("unable to parse minor version from tag %q", vt)
		}
		versions.Insert(releaseVer{major: major, minor: minor})
	}

	return versions, nil
}

func listVersionsForChannel(ctx context.Context, log clog.PluggableLoggerInterface, w io.Writer, remoteURL string, opts *listReleasesOptions) error {
	if strings.HasPrefix(opts.channel, "stable") {
		fmt.Fprintln(w, "Listing stable channels. Use --channel=<name> to filter.")
		fmt.Fprintf(w, "Use oc-mirror --v2 list releases --channels --version=%s to discover other channels.\n", opts.version)
		fmt.Fprintln(w, "")
	}

	var errs []error
	for _, arch := range opts.filterArchs {
		graph, err := loadGraphData(ctx, log, remoteURL, arch, opts.channel)
		if err != nil {
			errs = append(errs, fmt.Errorf("channel %q: %w", opts.channel, err))
			continue
		}
		vers := graph.GetVersions(nil)
		if len(vers) == 0 {
			errs = append(errs, fmt.Errorf("channel %q: no versions found", opts.channel))
			continue
		}

		fmt.Fprintf(w, "Channel: %s\nArchitecture: %s\n", opts.channel, arch)
		for _, ver := range vers {
			fmt.Fprintf(w, "%s\n", ver)
		}

	}

	return errors.Join(errs...)
}

func loadGraphData(ctx context.Context, log clog.PluggableLoggerInterface, remote, arch, channel string) (*cincinnati.Graph, error) {
	data, err := cincinnati.DownloadGraphData(ctx, log, cincinnati.WithArch(arch), cincinnati.WithChannel(channel), cincinnati.WithURL(remote))
	if err != nil {
		return nil, fmt.Errorf("failed to get graph data: %w", err)
	}

	graph, err := cincinnati.LoadGraphData(data)
	if err != nil {
		return nil, err
	}

	return &graph, nil
}
