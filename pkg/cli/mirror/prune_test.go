package mirror

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc/pkg/cli/admin/prune/imageprune"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestPlanImagePruning(t *testing.T) {
	type spec struct {
		desc       string
		opts       *MirrorOptions
		curr       image.AssociationSet
		prev       image.AssociationSet
		expImages  map[string]string
		expError   string
		assertFunc func(manifestDeleter) bool
	}

	cases := []spec{
		{
			desc: "Success/FirstRun",
			opts: &MirrorOptions{
				RootOptions:   &cli.RootOptions{},
				ToMirror:      "test-registry.com",
				UserNamespace: "namespace",
			},
			prev: image.AssociationSet{},
			curr: image.AssociationSet{"source-reg.com/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": image.Associations{
				"source-reg.com/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": {
					Name:            "source-reg.com/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Path:            "single_manifest",
					TagSymlink:      "latest",
					ID:              "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Type:            v1alpha2.TypeGeneric,
					ManifestDigests: nil,
					LayerDigests: []string{
						"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
						"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
						"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
						"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
						"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
						"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
					},
				},
			},
			},
			expImages: map[string]string{},
			assertFunc: func(deleter manifestDeleter) bool {
				return deleter.registry == "test-registry.com"
			},
		},
		{
			desc: "Success/DifferentialRun",
			opts: &MirrorOptions{
				RootOptions:   &cli.RootOptions{},
				ToMirror:      "test-registry.com",
				UserNamespace: "namespace",
			},
			prev: image.AssociationSet{
				"source-reg.com/test/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": image.Associations{
					"source-reg.com/test/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": {
						Name:            "source-reg.com/test/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
						Path:            "single_manifest",
						TagSymlink:      "latest",
						ID:              "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
						Type:            v1alpha2.TypeGeneric,
						ManifestDigests: nil,
						LayerDigests: []string{
							"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
							"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
							"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
							"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
							"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
							"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
						},
					},
				},
				"source-reg.com/test/imgname@sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b": image.Associations{
					"source-reg.com/test/imgname@sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b": {
						Name:            "source-reg.com/test/imgname@sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
						Path:            "single_manifest",
						TagSymlink:      "latest",
						ID:              "sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
						Type:            v1alpha2.TypeGeneric,
						ManifestDigests: nil,
						LayerDigests: []string{
							"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
							"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
							"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
							"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
							"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
							"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
						},
					},
				},
			},
			curr: image.AssociationSet{"source-reg.com/test/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": image.Associations{
				"source-reg.com/test/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": {
					Name:            "source-reg.com/test/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Path:            "single_manifest",
					TagSymlink:      "latest",
					ID:              "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Type:            v1alpha2.TypeGeneric,
					ManifestDigests: nil,
					LayerDigests: []string{
						"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
						"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
						"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
						"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
						"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
						"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
					},
				},
			},
			},
			expImages: map[string]string{
				"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b": "namespace/single_manifest",
			},
			assertFunc: func(deleter manifestDeleter) bool {
				return deleter.registry == "test-registry.com"
			},
		},
		{
			desc: "Success/ImagesWithTagsOnly",
			opts: &MirrorOptions{
				RootOptions:   &cli.RootOptions{},
				ToMirror:      "test-registry.com",
				UserNamespace: "namespace",
			},
			prev: image.AssociationSet{
				"source-reg.com/test/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": image.Associations{
					"source-reg.com/test/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": {
						Name:            "source-reg.com/test/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
						Path:            "single_manifest",
						TagSymlink:      "latest",
						Type:            v1alpha2.TypeGeneric,
						ManifestDigests: nil,
						LayerDigests: []string{
							"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
							"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
							"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
							"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
							"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
							"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
						},
					},
				},
				"source-reg.com/test/imgname@sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b": image.Associations{
					"source-reg.com/test/imgname@sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b": {
						Name:            "source-reg.com/test/imgname@sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
						Path:            "single_manifest",
						TagSymlink:      "latest",
						Type:            v1alpha2.TypeGeneric,
						ManifestDigests: nil,
						LayerDigests: []string{
							"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
							"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
							"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
							"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
							"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
							"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
						},
					},
				},
			},
			curr: image.AssociationSet{"source-reg.com/test/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": image.Associations{
				"source-reg.com/test/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": {
					Name:            "source-reg.com/test/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Path:            "single_manifest",
					TagSymlink:      "latest",
					Type:            v1alpha2.TypeGeneric,
					ManifestDigests: nil,
					LayerDigests: []string{
						"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
						"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
						"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
						"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
						"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
						"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
					},
				},
			},
			},
			expImages: map[string]string{},
			assertFunc: func(deleter manifestDeleter) bool {
				return deleter.registry == "test-registry.com"
			},
		},
		{
			desc: "Success/AssocImagePath",
			opts: &MirrorOptions{
				RootOptions:   &cli.RootOptions{},
				ToMirror:      "test-registry.com",
				UserNamespace: "namespace",
			},
			prev: image.AssociationSet{
				"source-reg.com/test/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": image.Associations{
					"source-reg.com/test/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": {
						Name:            "source-reg.com/test/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
						Path:            "test-registry.com/namespace/single_manifest:latest",
						TagSymlink:      "latest",
						ID:              "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
						Type:            v1alpha2.TypeGeneric,
						ManifestDigests: nil,
						LayerDigests: []string{
							"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
							"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
							"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
							"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
							"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
							"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
						},
					},
				},
				"source-reg.com/test/imgname@sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b": image.Associations{
					"source-reg.com/test/imgname@sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b": {
						Name:            "source-reg.com/test/imgname@sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
						Path:            "test-registry.com/namespace/single_manifest:latest",
						TagSymlink:      "latest",
						ID:              "sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
						Type:            v1alpha2.TypeGeneric,
						ManifestDigests: nil,
						LayerDigests: []string{
							"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
							"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
							"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
							"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
							"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
							"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
						},
					},
				},
			},
			curr: image.AssociationSet{"source-reg.com/test/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": image.Associations{
				"source-reg.com/test/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": {
					Name:            "source-reg.com/test/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Path:            "test-registry.com/single_manifest:latest",
					TagSymlink:      "latest",
					ID:              "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Type:            v1alpha2.TypeGeneric,
					ManifestDigests: nil,
					LayerDigests: []string{
						"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
						"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
						"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
						"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
						"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
						"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
					},
				},
			},
			},
			expImages: map[string]string{
				"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b": "namespace/single_manifest",
			},
			assertFunc: func(deleter manifestDeleter) bool {
				return deleter.registry == "test-registry.com"
			},
		},
		{
			desc: "Failure/InvalidAssociations",
			opts: &MirrorOptions{
				RootOptions:   &cli.RootOptions{},
				ToMirror:      "test-registry.com",
				UserNamespace: "namespace",
			},
			prev: image.AssociationSet{},
			curr: image.AssociationSet{"source-reg.com/imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": image.Associations{
				"imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": {
					Name:            "imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Path:            "single_manifest",
					TagSymlink:      "latest",
					ID:              "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Type:            v1alpha2.TypeGeneric,
					ManifestDigests: nil,
					LayerDigests: []string{
						"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
						"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
						"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
						"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
						"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
						"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
					},
				},
			},
			},
			expImages: map[string]string{},
			assertFunc: func(deleter manifestDeleter) bool {
				return deleter.registry == "test-registry.com"
			},
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			deleter, images, err := c.opts.planImagePruning(context.TODO(), c.curr, c.prev)
			if c.expError != "" {
				require.EqualError(t, err, c.expError)
			} else {
				require.NoError(t, err)
				concreteDeleter, ok := deleter.(*manifestDeleter)
				require.True(t, ok)
				require.True(t, c.assertFunc(*concreteDeleter))
				require.Equal(t, c.expImages, images)
			}
		})
	}
}

func TestPruneImages(t *testing.T) {
	type spec struct {
		desc          string
		images        map[string]string
		expInvocation int
		exp           []string
	}

	cases := []spec{
		{
			desc:          "Success/OneImagePruned",
			images:        map[string]string{"digest": "repo"},
			expInvocation: 1,
			exp:           []string{"repo|digest"},
		},
		{
			desc: "Success/FiveImagesPruned",
			images: map[string]string{
				"digest1": "repo1",
				"digest2": "repo2",
				"digest3": "repo3",
				"digest4": "repo4",
				"digest5": "repo5",
			},
			expInvocation: 5,
			exp: []string{
				"repo1|digest1",
				"repo2|digest2",
				"repo3|digest3",
				"repo4|digest4",
				"repo5|digest5",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			manifestDeleter := &fakeManifestDeleter{invocations: sets.NewString()}
			err := pruneImages(manifestDeleter, c.images, 2)
			require.NoError(t, err)
			require.Equal(t, c.expInvocation, manifestDeleter.invocations.Len())
			t.Log(manifestDeleter.invocations.List())
			require.True(t, manifestDeleter.invocations.HasAll(c.exp...))
		})
	}
}

type fakeManifestDeleter struct {
	mutex       sync.Mutex
	invocations sets.String
	err         error
}

var _ imageprune.ManifestDeleter = &fakeManifestDeleter{}

func (p *fakeManifestDeleter) DeleteManifest(repo, manifest string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.invocations.Insert(fmt.Sprintf("%s|%s", repo, manifest))
	return p.err
}
