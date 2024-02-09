package additional

import (
	"context"
	"testing"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

// setup mocks
// we need to mock Manifest, Mirror

type MockMirror struct{}
type MockManifest struct {
	Log clog.PluggableLoggerInterface
}

func TestAdditionalImageCollector(t *testing.T) {
	log := clog.New("trace")

	global := &mirror.GlobalOptions{TlsVerify: false, SecurePolicy: false}
	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	opts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Destination:         "oci://test",
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
	}

	// use the minamal amount of images
	// simplifies testing
	cfg := v1alpha2.ImageSetConfiguration{
		ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
			Mirror: v1alpha2.Mirror{
				AdditionalImages: []v1alpha2.Image{
					{Name: "registry.redhat.io/ubi8/ubi:latest"},
					{Name: "sometest.registry.com/testns/test@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
					{Name: "oci://sometest.registry.com/testns/test@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
				},
			},
		},
	}

	mockmirror := MockMirror{}
	manifest := MockManifest{Log: log}
	localstorageFQDN := "test.registry.com"
	ex := New(log, cfg, opts, mockmirror, manifest, localstorageFQDN)
	ctx := context.Background()

	// this test covers mirrorToDisk
	t.Run("Testing AdditionalImagesCollector : mirrorToDisk should pass", func(t *testing.T) {
		res, err := ex.AdditionalImagesCollector(ctx)
		if err != nil {
			log.Error(" %v ", err)
			t.Fatalf("should not fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	// update opts
	// this test covers diskToMirror
	opts.Mode = mirror.DiskToMirror
	ex = New(log, cfg, opts, mockmirror, manifest, localstorageFQDN)

	t.Run("Testing AdditionalImagesCollector : diskToMirror should pass", func(t *testing.T) {
		res, err := ex.AdditionalImagesCollector(ctx)
		if err != nil {
			log.Error(" %v ", err)
			t.Fatalf("should not fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	// should error mirrorToDisk
	cfg.Mirror.AdditionalImages[1].Name = "sometest.registry.com/testns/test@shaf30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"
	opts.Mode = mirror.MirrorToDisk
	ex = New(log, cfg, opts, mockmirror, manifest, localstorageFQDN)

	t.Run("Testing AdditionalImagesCollector : mirrorToDisk should fail", func(t *testing.T) {
		_, err := ex.AdditionalImagesCollector(ctx)
		if err == nil {
			t.Fatalf("should fail")
		}
	})

	// should error diskToMirror
	cfg.Mirror.AdditionalImages[1].Name = "sometest.registry.com/testns/test@shaf30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"
	opts.Mode = mirror.DiskToMirror
	ex = New(log, cfg, opts, mockmirror, manifest, localstorageFQDN)

	t.Run("Testing AdditionalImagesCollector : diskToMirror should fail", func(t *testing.T) {
		_, err := ex.AdditionalImagesCollector(ctx)
		if err == nil {
			t.Fatalf("should fail")
		}
	})
}

func (o MockMirror) Run(ctx context.Context, src, dest string, mode mirror.Mode, opts *mirror.CopyOptions) error {
	return nil
}

func (o MockMirror) Check(ctx context.Context, image string, opts *mirror.CopyOptions) (bool, error) {
	return true, nil
}

func (o MockManifest) GetOperatorConfig(file string) (*v1alpha3.OperatorConfigSchema, error) {
	opcl := v1alpha3.OperatorLabels{OperatorsOperatorframeworkIoIndexConfigsV1: "/configs"}
	opc := v1alpha3.OperatorConfig{Labels: opcl}
	ocs := &v1alpha3.OperatorConfigSchema{Config: opc}
	return ocs, nil
}

func (o MockManifest) GetReleaseSchema(filePath string) ([]v1alpha3.RelatedImage, error) {
	relatedImages := []v1alpha3.RelatedImage{
		{Name: "testA", Image: "sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testB", Image: "sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testC", Image: "sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testD", Image: "sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
	}
	return relatedImages, nil
}

func (o MockManifest) GetImageIndex(name string) (*v1alpha3.OCISchema, error) {
	return &v1alpha3.OCISchema{
		SchemaVersion: 2,
		Manifests: []v1alpha3.OCIManifest{
			{
				MediaType: "application/vnd.oci.image.manifest.v1+json",
				Digest:    "sha256:3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419",
				Size:      567,
			},
		},
	}, nil
}

func (o MockManifest) GetImageManifest(name string) (*v1alpha3.OCISchema, error) {
	return &v1alpha3.OCISchema{
		SchemaVersion: 2,
		Manifests: []v1alpha3.OCIManifest{
			{
				MediaType: "application/vnd.oci.image.manifest.v1+json",
				Digest:    "sha256:3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419",
				Size:      567,
			},
		},
		Config: v1alpha3.OCIManifest{
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Digest:    "sha256:3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419",
			Size:      567,
		},
	}, nil
}

func (o MockManifest) GetCatalog(filePath string) (manifest.OperatorCatalog, error) {
	return manifest.OperatorCatalog{}, nil
}

func (o MockManifest) GetRelatedImagesFromCatalog(operatorCatalog manifest.OperatorCatalog, op v1alpha2.Operator) (map[string][]v1alpha3.RelatedImage, error) {
	relatedImages := make(map[string][]v1alpha3.RelatedImage)
	relatedImages["abc"] = []v1alpha3.RelatedImage{
		{Name: "testA", Image: "quay.io/name/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testB", Image: "quay.io/name/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
	}
	return relatedImages, nil
}

func (o MockManifest) ExtractLayersOCI(filePath, toPath, label string, oci *v1alpha3.OCISchema) error {
	return nil
}

func (o MockManifest) ExtractLayers(filePath, name, label string) error {
	return nil
}

func (o MockManifest) ConvertIndexToSingleManifest(dir string, oci *v1alpha3.OCISchema) error {
	return nil
}
