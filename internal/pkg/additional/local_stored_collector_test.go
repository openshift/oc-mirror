package additional

import (
	"context"
	"testing"

	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

// setup mocks
// we need to mock Manifest, Mirror

type MockMirror struct{}
type MockManifest struct {
	Log clog.PluggableLoggerInterface
}

func TestAdditionalImageCollector(t *testing.T) {
	log := clog.New("trace")

	global := &mirror.GlobalOptions{SecurePolicy: false}
	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	localstorageFQDN := "test.registry.com"

	opts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Destination:         "oci://test",
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
		LocalStorageFQDN:    localstorageFQDN,
	}

	// use the minamal amount of images
	// simplifies testing
	cfg := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				AdditionalImages: []v2alpha1.Image{
					{Name: "registry.redhat.io/ubi8/ubi:latest"},
					{Name: "registry.redhat.io/ubi8/ubi:latest@sha256:44d75007b39e0e1bbf1bcfd0721245add54c54c3f83903f8926fb4bef6827aa2"},
					{Name: "sometest.registry.com/testns/test@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
					{Name: "oci:///folder-a/folder-b/testns/test"},
				},
			},
		},
	}

	mockmirror := MockMirror{}
	manifest := MockManifest{Log: log}

	ex := New(log, cfg, opts, mockmirror, manifest)
	ctx := context.Background()

	// this test covers mirrorToDisk
	t.Run("Testing AdditionalImagesCollector : mirrorToDisk should pass", func(t *testing.T) {
		expected := []v2alpha1.CopyImageSchema{
			{
				Source:      "docker://registry.redhat.io/ubi8/ubi:latest",
				Origin:      "registry.redhat.io/ubi8/ubi:latest",
				Destination: "docker://test.registry.com/ubi8/ubi:latest",
				Type:        v2alpha1.TypeGeneric,
			},
			{
				Source:      "docker://registry.redhat.io/ubi8/ubi@sha256:44d75007b39e0e1bbf1bcfd0721245add54c54c3f83903f8926fb4bef6827aa2",
				Origin:      "registry.redhat.io/ubi8/ubi:latest@sha256:44d75007b39e0e1bbf1bcfd0721245add54c54c3f83903f8926fb4bef6827aa2",
				Destination: "docker://test.registry.com/ubi8/ubi:latest",
				Type:        v2alpha1.TypeGeneric,
			},
			{
				Source:      "docker://sometest.registry.com/testns/test@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Origin:      "sometest.registry.com/testns/test@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Destination: "docker://test.registry.com/testns/test:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Type:        v2alpha1.TypeGeneric,
			},
			{
				Source:      "oci:///folder-a/folder-b/testns/test",
				Origin:      "oci:///folder-a/folder-b/testns/test",
				Destination: "docker://test.registry.com/folder-a/folder-b/testns/test:latest",
				Type:        v2alpha1.TypeGeneric,
			},
		}
		res, err := ex.AdditionalImagesCollector(ctx)
		if err != nil {
			log.Error(" %v ", err)
			t.Fatalf("should not fail")
		}
		assert.ElementsMatch(t, expected, res)
	})

	// update opts
	// this test covers diskToMirror
	opts.Mode = mirror.DiskToMirror
	opts.Destination = "docker://mirror.acme.com"
	ex = New(log, cfg, opts, mockmirror, manifest)

	t.Run("Testing AdditionalImagesCollector : diskToMirror should pass", func(t *testing.T) {
		expected := []v2alpha1.CopyImageSchema{
			{
				Destination: "docker://mirror.acme.com/ubi8/ubi:latest",
				Origin:      "registry.redhat.io/ubi8/ubi:latest",
				Source:      "docker://test.registry.com/ubi8/ubi:latest",
				Type:        v2alpha1.TypeGeneric,
			},
			{
				Destination: "docker://mirror.acme.com/ubi8/ubi:latest",
				Origin:      "registry.redhat.io/ubi8/ubi:latest@sha256:44d75007b39e0e1bbf1bcfd0721245add54c54c3f83903f8926fb4bef6827aa2",
				Source:      "docker://test.registry.com/ubi8/ubi:latest",
				Type:        v2alpha1.TypeGeneric,
			},
			{
				Destination: "docker://mirror.acme.com/testns/test:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Origin:      "sometest.registry.com/testns/test@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Source:      "docker://test.registry.com/testns/test:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Type:        v2alpha1.TypeGeneric,
			},
			{
				Destination: "docker://mirror.acme.com/folder-a/folder-b/testns/test:latest",
				Origin:      "oci:///folder-a/folder-b/testns/test",
				Source:      "docker://test.registry.com/folder-a/folder-b/testns/test:latest",
				Type:        v2alpha1.TypeGeneric,
			},
		}
		res, err := ex.AdditionalImagesCollector(ctx)
		if err != nil {
			log.Error(" %v ", err)
			t.Fatalf("should not fail")
		}
		assert.ElementsMatch(t, expected, res)
	})

	t.Run("Testing AdditionalImagesCollector : diskToMirror with generateV1Tags should use latest for images by digest", func(t *testing.T) {
		// should error diskToMirror
		opts.Mode = mirror.DiskToMirror
		ex = New(log, cfg, opts, mockmirror, manifest)
		ex = WithV1Tags(ex)
		expected := []v2alpha1.CopyImageSchema{
			{
				Destination: "docker://mirror.acme.com/ubi8/ubi:latest",
				Origin:      "registry.redhat.io/ubi8/ubi:latest",
				Source:      "docker://test.registry.com/ubi8/ubi:latest",
				Type:        v2alpha1.TypeGeneric,
			},
			{
				Destination: "docker://mirror.acme.com/ubi8/ubi:latest",
				Origin:      "registry.redhat.io/ubi8/ubi:latest@sha256:44d75007b39e0e1bbf1bcfd0721245add54c54c3f83903f8926fb4bef6827aa2",
				Source:      "docker://test.registry.com/ubi8/ubi:latest",
				Type:        v2alpha1.TypeGeneric,
			},
			{
				Destination: "docker://mirror.acme.com/testns/test:latest",
				Origin:      "sometest.registry.com/testns/test@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Source:      "docker://test.registry.com/testns/test:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Type:        v2alpha1.TypeGeneric,
			},
			{
				Destination: "docker://mirror.acme.com/folder-a/folder-b/testns/test:latest",
				Origin:      "oci:///folder-a/folder-b/testns/test",
				Source:      "docker://test.registry.com/folder-a/folder-b/testns/test:latest",
				Type:        v2alpha1.TypeGeneric,
			},
		}
		res, err := ex.AdditionalImagesCollector(ctx)
		if err != nil {
			log.Error(" %v ", err)
			t.Fatalf("should not fail")
		}
		assert.ElementsMatch(t, expected, res)
	})

	// should error mirrorToDisk
	cfg.Mirror.AdditionalImages[1].Name = "sometest.registry.com/testns/test@shaf30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"
	opts.Mode = mirror.MirrorToDisk
	ex = New(log, cfg, opts, mockmirror, manifest)

	t.Run("Testing AdditionalImagesCollector : mirrorToDisk should not fail (skipped)", func(t *testing.T) {
		expected := []v2alpha1.CopyImageSchema{
			{
				Source:      "docker://registry.redhat.io/ubi8/ubi:latest",
				Origin:      "registry.redhat.io/ubi8/ubi:latest",
				Destination: "docker://test.registry.com/ubi8/ubi:latest",
				Type:        v2alpha1.TypeGeneric,
			},
			{
				Source:      "docker://sometest.registry.com/testns/test@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Origin:      "sometest.registry.com/testns/test@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Destination: "docker://test.registry.com/testns/test:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Type:        v2alpha1.TypeGeneric,
			},
			{
				Source:      "oci:///folder-a/folder-b/testns/test",
				Origin:      "oci:///folder-a/folder-b/testns/test",
				Destination: "docker://test.registry.com/folder-a/folder-b/testns/test:latest",
				Type:        v2alpha1.TypeGeneric,
			},
		}
		res, err := ex.AdditionalImagesCollector(ctx)
		if err != nil {
			log.Error(" %v ", err)
			t.Fatalf("should not fail")
		}
		assert.ElementsMatch(t, expected, res)
	})

	t.Run("Testing AdditionalImagesCollector : diskToMirror should skip failing image with warning", func(t *testing.T) {
		// should error diskToMirror
		cfg.Mirror.AdditionalImages[1].Name = "sometest.registry.com/testns/test@shaf30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"
		opts.Mode = mirror.DiskToMirror
		ex = New(log, cfg, opts, mockmirror, manifest)
		expected := []v2alpha1.CopyImageSchema{
			{
				Destination: "docker://mirror.acme.com/ubi8/ubi:latest",
				Origin:      "registry.redhat.io/ubi8/ubi:latest",
				Source:      "docker://test.registry.com/ubi8/ubi:latest",
				Type:        v2alpha1.TypeGeneric,
			},
			{
				Destination: "docker://mirror.acme.com/testns/test:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Origin:      "sometest.registry.com/testns/test@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Source:      "docker://test.registry.com/testns/test:sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
				Type:        v2alpha1.TypeGeneric,
			},
			{
				Destination: "docker://mirror.acme.com/folder-a/folder-b/testns/test:latest",
				Origin:      "oci:///folder-a/folder-b/testns/test",
				Source:      "docker://test.registry.com/folder-a/folder-b/testns/test:latest",
				Type:        v2alpha1.TypeGeneric,
			},
		}
		res, err := ex.AdditionalImagesCollector(ctx)
		if err != nil {
			log.Error(" %v ", err)
			t.Fatalf("should not fail")
		}
		assert.ElementsMatch(t, expected, res)
	})
}

func (o MockMirror) Run(ctx context.Context, src, dest string, mode mirror.Mode, opts *mirror.CopyOptions) error {
	return nil
}

func (o MockMirror) Check(ctx context.Context, image string, opts *mirror.CopyOptions, asCopySrc bool) (bool, error) {
	return true, nil
}

func (o MockManifest) GetOperatorConfig(file string) (*v2alpha1.OperatorConfigSchema, error) {
	opcl := v2alpha1.OperatorLabels{OperatorsOperatorframeworkIoIndexConfigsV1: "/configs"}
	opc := v2alpha1.OperatorConfig{Labels: opcl}
	ocs := &v2alpha1.OperatorConfigSchema{Config: opc}
	return ocs, nil
}

func (o MockManifest) GetReleaseSchema(filePath string) ([]v2alpha1.RelatedImage, error) {
	relatedImages := []v2alpha1.RelatedImage{
		{Name: "testA", Image: "sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testB", Image: "sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testC", Image: "sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testD", Image: "sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
	}
	return relatedImages, nil
}

func (o MockManifest) GetOCIImageIndex(name string) (*v2alpha1.OCISchema, error) {
	return &v2alpha1.OCISchema{
		SchemaVersion: 2,
		Manifests: []v2alpha1.OCIManifest{
			{
				MediaType: "application/vnd.oci.image.manifest.v1+json",
				Digest:    "sha256:3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419",
				Size:      567,
			},
		},
	}, nil
}

func (o MockManifest) GetOCIImageManifest(name string) (*v2alpha1.OCISchema, error) {
	return &v2alpha1.OCISchema{
		SchemaVersion: 2,
		Manifests: []v2alpha1.OCIManifest{
			{
				MediaType: "application/vnd.oci.image.manifest.v1+json",
				Digest:    "sha256:3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419",
				Size:      567,
			},
		},
		Config: v2alpha1.OCIManifest{
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Digest:    "sha256:3ef0b0141abd1548f60c4f3b23ecfc415142b0e842215f38e98610a3b2e52419",
			Size:      567,
		},
	}, nil
}

func (o MockManifest) GetOCIImageFromIndex(dir string) (gcrv1.Image, error) { //nolint:ireturn // interface is expected here
	return nil, nil
}

func (o MockManifest) ExtractOCILayers(filePath, toPath, label string, oci *v2alpha1.OCISchema) error {
	return nil
}

func (o MockManifest) ExtractLayers(filePath, name, label string) error {
	return nil
}

func (o MockManifest) ConvertOCIIndexToSingleManifest(dir string, oci *v2alpha1.OCISchema) error {
	return nil
}

func (o MockManifest) ImageDigest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string) (string, error) {
	return "123456", nil
}

func (o MockManifest) ImageManifest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string, instanceDigest *digest.Digest) ([]byte, string, error) {
	return nil, "", nil
}
