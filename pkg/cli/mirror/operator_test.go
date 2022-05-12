package mirror

import (
	"context"
	"fmt"
	"testing"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/reference"
	"github.com/containerd/containerd/remotes"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPinImages(t *testing.T) {

	type spec struct {
		desc        string
		opts        *OperatorOptions
		dc          *declcfg.DeclarativeConfig
		resolver    remotes.Resolver
		expErrorStr string
	}

	cases := []spec{
		{
			desc: "Success/Resolved",
			opts: &OperatorOptions{
				MirrorOptions: &MirrorOptions{
					ContinueOnError: false,
					SkipMissing:     false,
				},
			},
			dc: &declcfg.DeclarativeConfig{
				Bundles: []declcfg.Bundle{
					{
						Name:  "foo.v1.0.0",
						Image: "regx1203109.com/ns/exist-bundle:latest",
						RelatedImages: []declcfg.RelatedImage{
							{Name: "relatedimage1", Image: "regx1203109.com/ns/exist-relatedimage:latest"},
						},
					},
					{
						Name:  "bar.v1.0.0",
						Image: "regx1203109.com/ns/exist-bundle-notag",
						RelatedImages: []declcfg.RelatedImage{
							{Name: "relatedimage1", Image: "regx1203109.com/ns/exist-relatedimage-notag"},
						},
					},
				},
			},
			resolver: mockResolver{
				digestMapping: map[string]string{
					"regx1203109.com/ns/exist-bundle:latest":       "sha256:1234",
					"regx1203109.com/ns/exist-relatedimage:latest": "sha256:5678",
				},
			},
		},
		{
			desc: "Error/NotFound",
			opts: &OperatorOptions{
				MirrorOptions: &MirrorOptions{
					ContinueOnError: false,
					SkipMissing:     false,
				},
			},
			dc: &declcfg.DeclarativeConfig{
				Bundles: []declcfg.Bundle{
					{
						Name:  "foo.v1.0.0",
						Image: "regx1203109.com/ns/notexist-bundle:latest",
						RelatedImages: []declcfg.RelatedImage{
							{Name: "relatedimage1", Image: "regx1203109.com/ns/notexist-relatedimage:latest"},
						},
					},
				},
			},
			resolver:    mockResolver{digestMapping: map[string]string{}},
			expErrorStr: "not found",
		},
		{
			desc: "Error/NilConfig",
			opts: &OperatorOptions{
				MirrorOptions: &MirrorOptions{
					ContinueOnError: false,
					SkipMissing:     false,
				},
			},
			dc:          nil,
			resolver:    mockResolver{digestMapping: map[string]string{}},
			expErrorStr: "bug: nil declarative config",
		},
		{
			desc: "Success/ContinueOnError",
			opts: &OperatorOptions{
				MirrorOptions: &MirrorOptions{
					ContinueOnError: true,
					SkipMissing:     false,
				},
			},
			dc: &declcfg.DeclarativeConfig{
				Bundles: []declcfg.Bundle{
					{
						Name:  "foo.v1.0.0",
						Image: "docker.io/library/notexist-bundle:latest",
						RelatedImages: []declcfg.RelatedImage{
							{Name: "relatedimage1", Image: "docker.io/library/notexist-relatedimage:latest"},
						},
					},
				},
			},
			resolver: mockResolver{digestMapping: map[string]string{}},
		},
		{
			desc: "Success/SkipMissing",
			opts: &OperatorOptions{
				MirrorOptions: &MirrorOptions{
					ContinueOnError: false,
					SkipMissing:     true,
				},
			},
			dc: &declcfg.DeclarativeConfig{
				Bundles: []declcfg.Bundle{
					{
						Name:  "foo.v1.0.0",
						Image: "docker.io/library/notexist-bundle:latest",
						RelatedImages: []declcfg.RelatedImage{
							{Name: "relatedimage1", Image: "docker.io/library/notexist-relatedimage:latest"},
						},
					},
				},
			},
			resolver: mockResolver{digestMapping: map[string]string{}},
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			ctx := context.TODO()
			err := c.opts.pinImages(ctx, c.dc, c.resolver)
			if c.expErrorStr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, c.expErrorStr)
			}
		})
	}

}

func TestVerifyOperatorPkgFound(t *testing.T) {

	mo := &MirrorOptions{
		RootOptions: &cli.RootOptions{},
	}
	o := NewOperatorOptions(mo)
	o.complete()
	hook := test.NewLocal(o.Logger.Logger)

	type testvopf struct {
		desc        string
		dic         action.DiffIncludeConfig
		dc          *declcfg.DeclarativeConfig
		logCount    int
		expErrorStr string
	}

	cases := []testvopf{
		{
			desc: "Requested operator package not found in DC",
			dic: action.DiffIncludeConfig{
				Packages: []action.DiffIncludePackage{
					{
						Name: "foo",
					},
				},
			},
			dc: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{
						Name: "bar",
					},
				},
			},
			logCount:    1,
			expErrorStr: "Operator foo was not found",
		},
		{
			desc: "Requested operator package found in DC",
			dic: action.DiffIncludeConfig{
				Packages: []action.DiffIncludePackage{
					{
						Name: "foo",
					},
				},
			},
			dc: &declcfg.DeclarativeConfig{
				Packages: []declcfg.Package{
					{
						Name: "foo",
					},
				},
			},
			logCount:    0,
			expErrorStr: "",
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			o.verifyOperatorPkgFound(c.dic, c.dc)

			assert.Equal(t, c.logCount, len(hook.AllEntries()))
			if c.logCount > 0 && len(hook.Entries) > 0 {
				assert.Contains(t, hook.LastEntry().Message, c.expErrorStr)
			}

		})
		hook.Reset()
	}
}

type mockResolver struct {
	digestMapping map[string]string
}

func (r mockResolver) Resolve(ctx context.Context, ref string) (name string, desc ocispec.Descriptor, err error) {
	if r.digestMapping == nil {
		panic("mockResolver has not been initialized")
	}

	spec, err := reference.Parse(ref)
	if err != nil {
		return name, desc, err
	}

	fmt.Printf("%#v\n", spec)
	if d, ok := r.digestMapping[spec.String()]; ok {
		desc.Digest = digest.Digest(d)
	} else {
		err = errdefs.ErrNotFound
	}

	return spec.String(), desc, err
}

func (r mockResolver) Fetcher(ctx context.Context, ref string) (remotes.Fetcher, error) {
	return nil, nil
}

func (r mockResolver) Pusher(ctx context.Context, ref string) (remotes.Pusher, error) {
	return nil, nil
}
