package additional

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	manifestmock "github.com/openshift/oc-mirror/v2/internal/pkg/manifest/mock"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	mirrormock "github.com/openshift/oc-mirror/v2/internal/pkg/mirror/mock"
)

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

	// setup mocks
	// we need to mock Manifest, Mirror
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mirrorMock := mirrormock.NewMockMirrorInterface(mockCtrl)
	manifestMock := manifestmock.NewMockManifestInterface(mockCtrl)

	ex := New(log, cfg, opts, mirrorMock, manifestMock)
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
	ex = New(log, cfg, opts, mirrorMock, manifestMock)

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
		ex = New(log, cfg, opts, mirrorMock, manifestMock)
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
	ex = New(log, cfg, opts, mirrorMock, manifestMock)

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
		ex = New(log, cfg, opts, mirrorMock, manifestMock)
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
