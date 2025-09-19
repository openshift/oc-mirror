package version

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/yaml"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

var (
	// commitFromGit is a constant representing the source version that
	// generated this build. It should be set during build via -ldflags.
	commitFromGit string
	// versionFromGit is a constant representing the version tag that
	// generated this build. It should be set during build via -ldflags.
	versionFromGit = "v0.0.0-unknown"
	// major version
	majorFromGit string
	// minor version
	minorFromGit string
	// build date in ISO8601 format, output of $(date -u +'%Y-%m-%dT%H:%M:%SZ')
	buildDate string
	// state of git tree, either "clean" or "dirty"
	gitTreeState string

	// releaseVersionPadded may be replaced in the binary with Release
	// Metadata: Version that overrides defaultVersion as a null-terminated
	// string within the allowed character length. This allows a distributor to
	// override the version without having to rebuild the source.
	releaseVersionPadded = "\x00_RELEASE_VERSION_LOCATION_\x00XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX\x00"
	releaseVersionPrefix = "\x00_RELEASE_VERSION_LOCATION_\x00"
	releaseVersionLength = len(releaseVersionPadded)
)

func Version() (string, error) {
	if strings.HasPrefix(releaseVersionPadded, releaseVersionPrefix) {
		return fmt.Sprintf("unreleased-%s", commitFromGit), nil
	}
	nullTerminator := strings.IndexByte(releaseVersionPadded, '\x00')
	if nullTerminator == -1 {
		// the binary has been altered, but we didn't find a null terminator within the release name constant which is an error
		return commitFromGit, fmt.Errorf("release name location was replaced but without a null terminator before %d bytes", releaseVersionLength)
	} else if nullTerminator > releaseVersionLength {
		// the binary has been altered, but the null terminator is *longer* than the constant encoded in the library
		return commitFromGit, fmt.Errorf("release name location contains no null-terminator and constant is corrupted")
	}
	releaseName := releaseVersionPadded[:nullTerminator]
	if len(releaseName) == 0 {
		// the binary has been altered, but the replaced release name is empty which is incorrect
		// the oc binary will not be pinned to Release Metadata: Version
		return commitFromGit, fmt.Errorf("release name was incorrectly replaced during extract")
	}
	return releaseName, nil
}

type Info struct {
	Major        string `json:"major"`
	Minor        string `json:"minor"`
	GitVersion   string `json:"gitVersion"`
	GitCommit    string `json:"gitCommit"`
	GitTreeState string `json:"gitTreeState"`
	BuildDate    string `json:"buildDate"`
	GoVersion    string `json:"goVersion"`
	Compiler     string `json:"compiler"`
	Platform     string `json:"platform"`
}

type VersionOptions struct {
	Output string
	Short  bool
	V2     bool
}

// VersionInfo is a struct for version information
type VersionInfo struct {
	ClientVersion *Info `json:"clientVersion,omitempty" yaml:"clientVersion,omitempty"`
}

func NewVersionCommand(log clog.PluggableLoggerInterface) *cobra.Command {
	o := VersionOptions{}

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Output version",
		Example: templates.Examples(`
			# Get oc-mirror version
			oc-mirror version
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.Validate(); err != nil {
				return err
			}

			return o.Run()
		},
	}

	fs := cmd.Flags()
	fs.BoolVar(&o.Short, "short", o.Short, "Print just the version number")
	// nolint: errcheck
	fs.MarkDeprecated("short", "and will be removed in a future release. Use oc-mirror version instead.")
	fs.StringVar(&o.Output, "output", o.Output, "One of 'yaml' or 'json'.")
	fs.BoolVar(&o.V2, "v2", o.V2, "Redirect the flow to oc-mirror v2 - V2 is still under development and it is not production ready.")
	// nolint: errcheck
	fs.MarkHidden("v2")
	cmd.PersistentFlags()

	return cmd
}

func (o *VersionOptions) Validate() error {
	if o.Output != "" && o.Output != "yaml" && o.Output != "json" {
		return errors.New(`--output must be 'yaml' or 'json'`)
	}

	return nil
}

func (o *VersionOptions) Run() error {
	var versionInfo VersionInfo

	clientVersion := Get()
	versionInfo.ClientVersion = &clientVersion

	switch o.Output {
	case "":
		if o.Short {
			fmt.Fprintf(os.Stdout, "Client Version: %s\n", clientVersion.Major)
		} else {
			fmt.Fprintf(os.Stderr, "WARNING: This version information is deprecated and will be replaced with the output from --short. Use --output=yaml|json to get the full version.\n")
			fmt.Fprintf(os.Stdout, "Client Version: %#v\n", clientVersion)
		}
	case "yaml":
		marshalled, err := yaml.Marshal(&versionInfo)
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, string(marshalled))
	case "json":
		marshalled, err := json.MarshalIndent(&versionInfo, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, string(marshalled))
	default:
		return fmt.Errorf("VersionOptions were not validated: --output=%q should have been rejected", o.Output)
	}

	return nil
}

func Get() Info {
	version, err := Version()
	if err != nil {
		panic(fmt.Errorf("could not assembler binary version: %w", err))
	}
	return Info{
		Major:        version,
		Minor:        version,
		GitCommit:    commitFromGit,
		GitVersion:   versionFromGit,
		GitTreeState: gitTreeState,
		BuildDate:    buildDate,
		GoVersion:    runtime.Version(),
		Compiler:     runtime.Compiler,
		Platform:     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}
