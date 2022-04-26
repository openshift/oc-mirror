package initcmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/blang/semver/v4"
	"strings"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/yaml"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/version"
)

var catalogBase = "registry.redhat.io/redhat/redhat-operator-index"

type InitOptions struct {
	*cli.RootOptions
	Output string // TODO rename: format? Be consistent with `version`
}

func NewInitCommand(f kcmdutil.Factory, ro *cli.RootOptions) *cobra.Command {
	o := InitOptions{
		RootOptions: ro,
	}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Output initial config template",
		Example: templates.Examples(`
			# Get oc-mirror initial config template
			oc-mirror init
			
			# Save oc-mirror initial config to a file
			oc-mirror init >imageset-config.yaml
		`),
		Run: func(cmd *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Validate())
			kcmdutil.CheckErr(o.Run(context.WithValue(cmd.Context(), "catalogBase", catalogBase)))
		},
	}

	fs := cmd.Flags()
	fs.StringVar(&o.Output, "output", o.Output, "One of 'yaml' or 'json'.")
	o.BindFlags(cmd.PersistentFlags())

	return cmd
}

func (o *InitOptions) Validate() error {
	if o.Output != "" && o.Output != "yaml" && o.Output != "json" {
		return errors.New(`--output must be 'yaml' or 'json'`)
	}
	return nil
}

func (o *InitOptions) Run(ctx context.Context) error {
	var err error

	releaseChannel, err := getReleaseChannelFromGit()
	if err != nil {
		return err
	}
	catalog, err := getCatalog(ctx.Value("catalogBase").(string))
	if err != nil {
		return err
	}
	customRegistry, err := o.promptCustomRegistry()
	if err != nil {
		return err
	}

	imageSetConfig := v1alpha2.ImageSetConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha2.ImageSetConfigurationKind,
			APIVersion: v1alpha2.GroupVersion.String(),
		},
		ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
			StorageConfig: v1alpha2.StorageConfig{
				Registry: nil,
				Local:    &v1alpha2.LocalConfig{Path: "./"},
			},
			Mirror: v1alpha2.Mirror{
				Platform: v1alpha2.Platform{
					Graph: false,
					Channels: []v1alpha2.ReleaseChannel{
						{
							Name: releaseChannel,
							// Default empty other fields
						},
					},
				},
				Operators: []v1alpha2.Operator{
					{
						IncludeConfig: v1alpha2.IncludeConfig{
							Packages: []v1alpha2.IncludePackage{
								{
									Name: "serverless-operator",
									Channels: []v1alpha2.IncludeChannel{
										{
											Name: "stable",
											//IncludeBundle: v1alpha2.IncludeBundle{},
										},
									},
									IncludeBundle: v1alpha2.IncludeBundle{},
								},
							},
						},
						//Catalog:          "registry.redhat.io/redhat/redhat-operator-index:v4.12",
						Catalog:          catalog,
						Full:             false,
						SkipDependencies: false,
					},
				},
				AdditionalImages: []v1alpha2.Image{
					{Name: "registry.redhat.io/ubi8/ubi:latest"}, // Just use UBI as the default
				},
				Helm:          v1alpha2.Helm{},
				BlockedImages: nil,
			},
		},
	}

	if "" != customRegistry {
		registry := &v1alpha2.RegistryConfig{
			ImageURL: customRegistry,
			SkipTLS:  false,
		}
		imageSetConfig.ImageSetConfigurationSpec.StorageConfig.Registry = registry
	}

	switch o.Output {
	case "":
		fallthrough
	case "yaml":
		marshalled, err := yaml.Marshal(&imageSetConfig)
		if err != nil {
			return err
		}
		fmt.Fprint(o.Out, string(marshalled))
	case "json":
		marshalled, err := json.MarshalIndent(&imageSetConfig, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(o.Out, string(marshalled))
	default:
		return fmt.Errorf("InitOptions were not validated: --output=%q should have been rejected", o.Output)
	}

	return nil
}

func getReleaseChannelFromGit() (string, error) {
	gitVersion := version.Get().GitVersion
	// Example: v4.1.1-g01d6cf7
	// Example: v4.2.0
	vers, err := semver.ParseTolerant(gitVersion)
	if err != nil {
		return "", fmt.Errorf("unable to parse oc-mirror version: %s ; %w", gitVersion, err)
	}
	releaseChannel := fmt.Sprintf("stable-%d.%d", vers.Major, vers.Minor)
	return releaseChannel, nil
}

func getCatalog(catalogBase string) (string, error) {
	versionMap, err := image.GetTagsFromImage(catalogBase)
	if err != nil {
		return "", fmt.Errorf("unable to get version map for init: %w", err)
	}
	catalogLatestVersionString := "v0.0"
	catalogLatestVersion, err := semver.ParseTolerant(catalogLatestVersionString)
	if err != nil {
		return "", fmt.Errorf("impossible error: unable to parse %s as version", catalogLatestVersionString)
	}
	for versionString, count := range versionMap {
		if count == 0 {
			continue
		}
		catalogVersion, err := semver.ParseTolerant(versionString)
		if err != nil {
			continue
		}
		if catalogVersion.GT(catalogLatestVersion) {
			catalogLatestVersionString = versionString
			catalogLatestVersion = catalogVersion
		}
	}
	catalog := fmt.Sprintf("%s:%s", catalogBase, catalogLatestVersionString)
	return catalog, nil
}

func (o *InitOptions) promptCustomRegistry() (string, error) {
	fmt.Fprintln(o.ErrOut, "Enter custom registry image URL or blank for none.")
	fmt.Fprintln(o.ErrOut, "Example: localhost:5000/test:latest") // Obvious placeholder to prevent using a bad default
	customRegistry, err := bufio.NewReader(o.In).ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("error reading custom registry image URL: %w", err)
	}
	customRegistry = strings.TrimSpace(customRegistry)
	return customRegistry, nil
}
