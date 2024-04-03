package version

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"

	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/yaml"
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
)

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

// Version is a struct for version information
type Version struct {
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
		Run: func(cmd *cobra.Command, args []string) {
			if err := o.Validate(); err != nil {
				log.Error(" %v ", err)
				os.Exit(1)
			}

			if err := o.Run(); err != nil {
				log.Error(" %v ", err)
				os.Exit(1)
			}
		},
	}

	fs := cmd.Flags()
	fs.BoolVar(&o.Short, "short", o.Short, "Print just the version number")
	fs.MarkDeprecated("short", "and will be removed in a future release. Use oc-mirror version instead.")
	fs.StringVar(&o.Output, "output", o.Output, "One of 'yaml' or 'json'.")
	fs.BoolVar(&o.V2, "v2", o.V2, "Redirect the flow to oc-mirror v2 - V2 is still under development and it is not production ready.")
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
	var versionInfo Version

	clientVersion := Get()
	versionInfo.ClientVersion = &clientVersion

	switch o.Output {
	case "":
		if o.Short {
			fmt.Fprintf(os.Stdout, "Client Version: %s\n", clientVersion.GitVersion)
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
	return Info{
		Major:        majorFromGit,
		Minor:        minorFromGit,
		GitCommit:    commitFromGit,
		GitVersion:   versionFromGit,
		GitTreeState: gitTreeState,
		BuildDate:    buildDate,
		GoVersion:    runtime.Version(),
		Compiler:     runtime.Compiler,
		Platform:     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}
