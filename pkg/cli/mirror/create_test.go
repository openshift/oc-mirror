package mirror

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/cli"
)

func TestCreate(t *testing.T) {
	path := t.TempDir()
	ctx := context.Background()

	img := v1alpha2.Image{Name: "quay.io/redhatgov/oc-mirror-dev:latest"}

	cfg := v1alpha2.ImageSetConfiguration{}
	cfg.Mirror.AdditionalImages = append(cfg.Mirror.AdditionalImages, img)

	opts := MirrorOptions{
		RootOptions: &cli.RootOptions{
			Dir:      path,
			LogLevel: 2,
			IOStreams: genericclioptions.IOStreams{
				In:     os.Stdin,
				Out:    os.Stdout,
				ErrOut: os.Stderr,
			},
		},
		OutputDir: path,
	}
	_, mappings, err := opts.Create(ctx, cfg)
	require.NoError(t, err)
	require.Len(t, mappings, 1)
}

func TestCreateOlmArtifactsForOCI(t *testing.T) {

	type spec struct {
		desc        string
		cfg         v1alpha2.ImageSetConfiguration
		expectedErr string
	}

	cases := []spec{
		{
			desc: "Success Scenario",
			cfg: v1alpha2.ImageSetConfiguration{
				TypeMeta: v1alpha2.NewMetadata().TypeMeta,
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Operators: []v1alpha2.Operator{
							{
								Catalog: "oci://" + testdata,
								IncludeConfig: v1alpha2.IncludeConfig{
									Packages: []v1alpha2.IncludePackage{
										{
											Name: "aws-load-balancer-operator",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			desc: "Success Scenario (no OCI catalog should be skipped) ",
			cfg: v1alpha2.ImageSetConfiguration{
				TypeMeta: v1alpha2.NewMetadata().TypeMeta,
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Operators: []v1alpha2.Operator{
							{
								Catalog: testdata,
								IncludeConfig: v1alpha2.IncludeConfig{
									Packages: []v1alpha2.IncludePackage{
										{
											Name: "aws-load-balancer-operator",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			desc: "Fail Scenario (invalid catalog) ",
			cfg: v1alpha2.ImageSetConfiguration{
				TypeMeta: v1alpha2.NewMetadata().TypeMeta,
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{
						Operators: []v1alpha2.Operator{
							{
								Catalog: "oci:",
								IncludeConfig: v1alpha2.IncludeConfig{
									Packages: []v1alpha2.IncludePackage{
										{
											Name: "aws-load-balancer-operator",
										},
									},
								},
							},
						},
					},
				},
			},
			expectedErr: "unable to get OCI Image from oci:: open index.json: no such file or directory",
		},
	}

	path := t.TempDir()
	ctx := context.Background()

	opts := MirrorOptions{
		RootOptions: &cli.RootOptions{
			Dir:      path,
			LogLevel: 2,
			IOStreams: genericclioptions.IOStreams{
				In:     os.Stdin,
				Out:    os.Stdout,
				ErrOut: os.Stderr,
			},
		},
		OutputDir: path,
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			err := opts.createOlmArtifactsForOCI(ctx, c.cfg)
			if c.expectedErr != "" {
				require.EqualError(t, err, c.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
