package initcmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/spf13/cobra"
	yamlv2 "gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/version"
)

type InitOptions struct {
	*cli.RootOptions
	Output      string
	catalogBase string
}

func NewInitCommand(f kcmdutil.Factory, ro *cli.RootOptions) *cobra.Command {
	o := InitOptions{
		RootOptions: ro,
		catalogBase: "registry.redhat.io/redhat/redhat-operator-index",
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
			kcmdutil.CheckErr(o.Run(cmd.Context()))
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
	catalog, err := getCatalog(o.catalogBase)
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
		marshalled, err := orderedYamlMarshal(&imageSetConfig)
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

// Key order doesn't matter to machines, but is nice for humans.
// oc-mirror init output is for humans.
// Issue with k8s yaml library: no support for MapSlice
// Our structs have json tags but not yaml tags, and I didn't want to add yaml tags everywhere,
// so the JSON marshaller is used to adapt those tags and key sorting happens here.
// This is less code than adding MarshalYAML with MapSlices to our types
func orderedYamlMarshal(obj interface{}) ([]byte, error) {
	bytes, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	var yamlObj map[string]interface{}
	err = yamlv2.Unmarshal(bytes, &yamlObj)
	if err != nil {
		return nil, err
	}
	slices := interfaceToMapSlice(yamlObj)
	yamlOrder := map[string]map[string]int{
		"": {
			"kind":          1,
			"apiVersion":    2,
			"storageConfig": 3,
			"mirror":        4,
		},
		"mirror": {
			"platform":         1,
			"operators":        2,
			"additionalImages": 3,
			"helm":             4,
		},
		"packages": {
			"name":            1,
			"startingVersion": 2,
			"channels":        3,
		},
		"operators": {
			"catalog":  1,
			"packages": 2,
		},
		"channels": {
			"name": -1,
		},
	}
	sortedSlices := deepMapKeySort(slices, yamlOrder, "")
	bytes, err = yamlv2.Marshal(&sortedSlices)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func interfaceToMapSlice(slice interface{}) yamlv2.MapSlice {
	mapSlice := yamlv2.MapSlice{}
	iter := reflect.ValueOf(slice).MapRange()
	for iter.Next() {
		mapSlice = append(mapSlice, yamlv2.MapItem{
			Key:   fmt.Sprintf("%v", iter.Key()),
			Value: iter.Value().Interface(),
		})
	}
	return mapSlice
}

// map[parent][child]index
func deepMapKeySort(slice interface{}, orders map[string]map[string]int, parentKey string) interface{} {
	switch slice.(type) {
	case yamlv2.MapSlice:
		slices := (slice).(yamlv2.MapSlice)
		for sliceIndex, mapItem := range slices {
			key := fmt.Sprintf("%v", mapItem.Key)
			switch reflect.ValueOf(mapItem.Value).Kind() {
			case reflect.Slice:
				slices[sliceIndex].Value = deepMapKeySort(mapItem.Value, orders, key)
			case reflect.Map:
				childMapSlice := interfaceToMapSlice(mapItem.Value)
				slices[sliceIndex].Value = deepMapKeySort(childMapSlice, orders, key)
			}
		}
		sort.Slice(slices, func(i, j int) bool {
			keyI := slices[i].Key.(string)
			keyJ := slices[j].Key.(string)
			return orders[parentKey][keyI] < orders[parentKey][keyJ]
		})
		return slices
	}
	switch reflect.ValueOf(slice).Kind() {
	case reflect.Slice:
		slice_ := slice.([]interface{})
		for i := range slice_ {
			slice_[i] = deepMapKeySort(slice_[i], orders, parentKey)
		}
		return slice
	case reflect.Map:
		mapSlice := interfaceToMapSlice(slice)
		return deepMapKeySort(mapSlice, orders, parentKey)
	}
	return slice
}
