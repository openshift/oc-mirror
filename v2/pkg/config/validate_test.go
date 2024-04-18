package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
)

func TestValidate(t *testing.T) {

	type spec struct {
		name     string
		config   *v1alpha2.ImageSetConfiguration
		expError string
	}

	cases := []spec{
		{
			name: "Valid/UniqueCatalogs",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Operators: []v1alpha2.Operator{
							{
								Catalog: "test-catalog1:latest",
							},
							{
								Catalog: "test-catalog2:latest",
							},
						},
					},
				},
			},
		},
		{
			name: "Valid/UniqueCatalogsWithTarget",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Operators: []v1alpha2.Operator{
							{
								Catalog:       "test-catalog:latest",
								TargetCatalog: "test1",
							},
							{
								Catalog:       "test-catalog:latest",
								TargetCatalog: "test2",
							},
						},
					},
				},
			},
		},
		{
			name: "Valid/UniqueCatalogsWithTargetCatalogAndTargetTag",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Operators: []v1alpha2.Operator{
							{
								Catalog:       "test-catalog:latest",
								TargetCatalog: "test1",
								TargetTag:     "v1.3",
							},
							{
								Catalog:       "test-catalog:latest",
								TargetCatalog: "test2",
								TargetTag:     "latest",
							},
						},
					},
				},
			},
		},
		{
			name: "Valid/UniqueReleaseChannels",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Architectures: []string{v1alpha2.DefaultPlatformArchitecture},
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name: "channel1",
								},
								{
									Name: "channel2",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Invalid/DuplicateCatalogs",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Operators: []v1alpha2.Operator{
							{
								Catalog: "test-catalog:latest",
							},
							{
								Catalog: "test-catalog:latest",
							},
						},
					},
				},
			},
			expError: "invalid configuration: catalog \"test-catalog:latest\": duplicate found in configuration",
		},
		{
			name: "Invalid/DuplicateCatalogsWithTarget",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Operators: []v1alpha2.Operator{
							{
								Catalog:       "test-catalog1:latest",
								TargetCatalog: "test",
							},
							{
								Catalog:       "test-catalog2:latest",
								TargetCatalog: "test",
							},
						},
					},
				},
			},
			expError: "invalid configuration: catalog \"test:latest\": duplicate found in configuration",
		},
		{
			name: "Invalid/CatalogWithTargetCatalogContainsTag",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Operators: []v1alpha2.Operator{
							{
								Catalog:       "test-catalog1:latest",
								TargetCatalog: "test:v1.3",
							},
						},
					},
				},
			},
			expError: "invalid configuration: targetCatalog: test:v1.3 - value is not valid. It should not contain a tag or a digest. It is expected to be composed of 1 or more path components separated by /, where each path component is a set of alpha-numeric and  regexp (?:[._]|__|[-]*). For more, see https://github.com/containers/image/blob/main/docker/reference/regexp.go",
		},
		{
			name: "Invalid/CatalogWithTargetCatalogContainsDigest",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Operators: []v1alpha2.Operator{
							{
								Catalog:       "test-catalog1:latest",
								TargetCatalog: "a/b/test@sha256:45df874",
							},
						},
					},
				},
			},
			expError: "invalid configuration: targetCatalog: a/b/test@sha256:45df874 - value is not valid. It should not contain a tag or a digest. It is expected to be composed of 1 or more path components separated by /, where each path component is a set of alpha-numeric and  regexp (?:[._]|__|[-]*). For more, see https://github.com/containers/image/blob/main/docker/reference/regexp.go",
		},
		{
			name: "Invalid/CatalogFilteringByChannelsAndBundles",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Operators: []v1alpha2.Operator{
							{
								Catalog: "test-catalog1:latest",
								IncludeConfig: v1alpha2.IncludeConfig{
									Packages: []v1alpha2.IncludePackage{
										{
											Name: "operator1",
											SelectedBundles: []v1alpha2.SelectedBundle{
												{
													Name: "operator1.v1.0.0",
												},
											},
											Channels: []v1alpha2.IncludeChannel{
												{
													Name: "stable-xyz",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expError: "invalid configuration: catalog \"test-catalog1:latest\": operator \"operator1\": mixing both filtering by bundles and filtering by channels or minVersion/maxVersion is not allowed",
		},
		{
			name: "Invalid/DuplicateChannels",
			config: &v1alpha2.ImageSetConfiguration{
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Platform: v1alpha2.Platform{
							Channels: []v1alpha2.ReleaseChannel{
								{
									Name: "channel",
								},
								{
									Name: "channel",
								},
							},
						},
					},
				},
			},
			expError: "invalid configuration: release channel \"channel\": duplicate found in configuration",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Validate(c.config)
			if c.expError != "" {
				require.EqualError(t, err, c.expError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
