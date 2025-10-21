package mirror

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
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
		expImages  map[string][]string
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
			expImages: map[string][]string{},
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
			expImages: map[string][]string{
				"namespace/single_manifest": {"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b"},
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
						Name:            "source-reg.com/test/imgname:latest",
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
						Name:            "source-reg.com/test/imgname:latest",
						Path:            "index_manifest",
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
					Name:            "source-reg.com/test/imgname:latest",
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
			expImages: map[string][]string{},
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
			},
			expImages: map[string][]string{
				"namespace/single_manifest": {"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b"},
			},
			assertFunc: func(deleter manifestDeleter) bool {
				return deleter.registry == "test-registry.com"
			},
		},
		{
			desc: "Success/WorkflowChange",
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
			expImages: map[string][]string{
				"namespace/single_manifest": {"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b"},
			},
			assertFunc: func(deleter manifestDeleter) bool {
				return deleter.registry == "test-registry.com"
			},
		},
		{
			desc: "Success/OldChildManifests",
			opts: &MirrorOptions{
				RootOptions:   &cli.RootOptions{},
				ToMirror:      "test-registry.com",
				UserNamespace: "namespace",
			},
			prev: image.AssociationSet{
				"source-reg.com/test/imgname:latest": image.Associations{
					"source-reg.com/test/imgname:latest": {
						Name:       "source-reg.com/test/imgname:latest",
						Path:       "test-registry.com/namespace/index_manifest:latest",
						TagSymlink: "latest",
						ID:         "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
						Type:       v1alpha2.TypeGeneric,
						ManifestDigests: []string{
							"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
							"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
						},
						LayerDigests: nil,
					},
					"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b": {
						Name:            "sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
						Path:            "test-registry.com/namespace/index_manifest:latest",
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
					"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241": {
						Name:            "sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
						Path:            "test-registry.com/namespace/index_manifest:latest",
						TagSymlink:      "latest",
						ID:              "sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
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
			curr: image.AssociationSet{
				"source-reg.com/test/imgname:latest": image.Associations{
					"source-reg.com/test/imgname:latest": {
						Name:       "source-reg.com/test/imgname:latest",
						Path:       "test-registry.com/namespace/index_manifest:latest",
						TagSymlink: "latest",
						ID:         "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df20",
						Type:       v1alpha2.TypeGeneric,
						ManifestDigests: []string{
							"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
							"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e242",
						},
						LayerDigests: nil,
					},
					"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b": {
						Name:            "sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
						Path:            "test-registry.com/namespace/index_manifest:latest",
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
					"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e242": {
						Name:            "sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e242",
						Path:            "test-registry.com/namespace/index_manifest:latest",
						TagSymlink:      "latest",
						ID:              "sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e242",
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
			expImages: map[string][]string{
				"namespace/index_manifest": {
					"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
					"sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
				},
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
			expImages: map[string][]string{},
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
		opts          *MirrorOptions
		images        map[string][]string
		expInvocation int
		exp           []string
	}

	cases := []spec{
		{
			desc:          "Success/OneImagePruned",
			images:        map[string][]string{"repo": {"digest"}},
			expInvocation: 1,
			exp:           []string{"repo|digest"},
		},
		{
			desc: "Success/FiveImagesPruned",
			images: map[string][]string{
				"repo1": {"digest1"},
				"repo2": {"digest2"},
				"repo3": {"digest3"},
				"repo4": {"digest4"},
				"repo5": {"digest5"},
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
		{
			desc: "Success/FiveImagesPrunedSameRepo",
			images: map[string][]string{
				"repo1": {"digest1", "digest2", "digest3", "digest4", "digest5"},
			},
			expInvocation: 5,
			exp: []string{
				"repo1|digest1",
				"repo1|digest2",
				"repo1|digest3",
				"repo1|digest4",
				"repo1|digest5",
			},
		},
		{
			desc: "Success/MissingImages",
			images: map[string][]string{
				"repo1": {"digest1", "digest2", "digest3", "missing1", "missing2"},
			},
			expInvocation: 5,
			exp: []string{
				"repo1|digest1",
				"repo1|digest2",
				"repo1|digest3",
				"repo1|missing1",
				"repo1|missing2",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			manifestDeleter := &fakeManifestDeleter{invocations: sets.NewString()}
			err := c.opts.pruneImages(manifestDeleter, c.images, 2)
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
	if strings.HasPrefix(manifest, "missing") {
		p.err = &transport.Error{StatusCode: 404}
	}
	return p.err
}

func TestAggregateImageInformation(t *testing.T) {
	type spec struct {
		desc     string
		registry string
		images   map[string][]string
		exp      pruneImagePlan
	}

	cases := []spec{
		{
			desc:     "Success/NoImagePruned",
			images:   map[string][]string{},
			registry: "test-registry",
			exp: pruneImagePlan{
				Registry: "test-registry",
			},
		},
		{
			desc: "Success/FiveManifestDifferentRepos",
			images: map[string][]string{
				"repo1": {"digest1"},
				"repo2": {"digest2"},
				"repo3": {"digest3"},
				"repo4": {"digest4"},
				"repo5": {"digest5"},
			},
			registry: "test-registry",
			exp: pruneImagePlan{
				Registry: "test-registry",
				Repositories: []repository{
					{
						Name:      "repo1",
						Manifests: []string{"digest1"},
					},
					{
						Name:      "repo2",
						Manifests: []string{"digest2"},
					},
					{
						Name:      "repo3",
						Manifests: []string{"digest3"},
					},
					{
						Name:      "repo4",
						Manifests: []string{"digest4"},
					},
					{
						Name:      "repo5",
						Manifests: []string{"digest5"},
					},
				},
			},
		},
		{
			desc: "Success/FiveManifestSameRepo",
			images: map[string][]string{
				"repo1": {"digest1", "digest2", "digest3", "digest4", "digest5"},
			},
			registry: "test-registry",
			exp: pruneImagePlan{
				Registry: "test-registry",
				Repositories: []repository{
					{
						Name: "repo1",
						Manifests: []string{
							"digest1",
							"digest2",
							"digest3",
							"digest4",
							"digest5",
						},
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			plan := aggregateImageInformation(c.registry, c.images)
			require.Equal(t, c.exp, plan)
		})
	}
}

func TestWritePruneImagePlan(t *testing.T) {
	exp := `{
 "registry": "test-registry",
 "repositories": [
  {
   "name": "repo",
   "manifests": [
    "digest1"
   ]
  }
 ]
}`

	outBuf := new(strings.Builder)
	plan := pruneImagePlan{
		Registry: "test-registry",
		Repositories: []repository{
			{
				Name: "repo",
				Manifests: []string{
					"digest1",
				},
			},
		},
	}
	err := writePruneImagePlan(outBuf, plan)
	require.NoError(t, err)
	require.Equal(t, exp, outBuf.String())
}

func TestPruneImages_WithError(t *testing.T) {
	type spec struct {
		desc          string
		opts          *MirrorOptions
		images        map[string][]string
		expInvocation int
		exp           []string
		expError      error
	}

	cases := []spec{
		{
			desc:          "Success/ContinueOnError",
			images:        map[string][]string{"repo": {"digest"}},
			expInvocation: 1,
			exp:           []string{"repo|digest"},
			opts: &MirrorOptions{
				ContinueOnError: true,
			},
		},
		{
			desc: "Failure/NoContinueOnError",
			opts: &MirrorOptions{
				ContinueOnError: false,
			},
			images: map[string][]string{
				"repo1": {"digest1"},
				"repo2": {"digest2"},
				"repo3": {"digest3"},
				"repo4": {"digest4"},
				"repo5": {"digest5"},
			},
			expInvocation: 0,
			exp:           []string{},
			expError:      &transport.Error{},
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			manifestDeleter := &failingManifestDeleter{invocations: sets.NewString()}
			err := c.opts.pruneImages(manifestDeleter, c.images, 2)
			if c.expError != nil {
				require.ErrorAs(t, err, &c.expError)
			} else {
				require.NoError(t, err)
				require.Equal(t, c.expInvocation, manifestDeleter.invocations.Len())
				t.Log(manifestDeleter.invocations.List())
				require.True(t, manifestDeleter.invocations.HasAll(c.exp...))
			}
		})
	}
}

type failingManifestDeleter struct {
	mutex       sync.Mutex
	invocations sets.String
	err         error
}

var _ imageprune.ManifestDeleter = &failingManifestDeleter{}

func (p *failingManifestDeleter) DeleteManifest(repo, manifest string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.invocations.Insert(fmt.Sprintf("%s|%s", repo, manifest))
	p.err = &transport.Error{StatusCode: 401}
	return p.err
}
