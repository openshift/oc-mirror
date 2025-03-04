package release

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/layout"
	"go.uber.org/mock/gomock"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/common"
	imgbuildermock "github.com/openshift/oc-mirror/v2/internal/pkg/imagebuilder/mock"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	manifestmock "github.com/openshift/oc-mirror/v2/internal/pkg/manifest/mock"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	mirrormock "github.com/openshift/oc-mirror/v2/internal/pkg/mirror/mock"
	releasemock "github.com/openshift/oc-mirror/v2/internal/pkg/release/mock"
)

func TestCreateGraphImage(t *testing.T) {
	log := clog.New("trace")
	globalM2D := &mirror.GlobalOptions{
		SecurePolicy: false,
		WorkingDir:   t.TempDir(),
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, retryOpts := mirror.RetryFlags()
	_, srcOptsM2D := mirror.ImageSrcFlags(globalM2D, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOptsM2D := mirror.ImageDestFlags(globalM2D, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")

	m2dOpts := mirror.CopyOptions{
		Global:              globalM2D,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOptsM2D,
		DestImage:           destOptsM2D,
		RetryOpts:           retryOpts,
		Destination:         "file://test",
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
		LocalStorageFQDN:    "localhost:9999",
	}

	cfgm2d := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Platform: v2alpha1.Platform{
					Graph: true,
					Channels: []v2alpha1.ReleaseChannel{
						{
							Name: "stable-4.7",
						},
						{
							Name:       "stable-4.6",
							MinVersion: "4.6.3",
							MaxVersion: "4.6.13",
						},
						{
							Name: "okd",
							Type: v2alpha1.TypeOKD,
						},
					},
				},
				Operators: []v2alpha1.Operator{
					{
						Catalog: "redhat-operators:v4.7",
						Full:    true,
					},
					{
						Catalog: "certified-operators:v4.7",
						Full:    true,
						IncludeConfig: v2alpha1.IncludeConfig{
							Packages: []v2alpha1.IncludePackage{
								{Name: "couchbase-operator"},
								{
									Name: "mongodb-operator",
									IncludeBundle: v2alpha1.IncludeBundle{
										MinVersion: "1.4.0",
									},
								},
								{
									Name: "crunchy-postgresql-operator",
									Channels: []v2alpha1.IncludeChannel{
										{Name: "stable"},
									},
								},
							},
						},
					},
					{
						Catalog: "community-operators:v4.7",
					},
				},
				AdditionalImages: []v2alpha1.Image{
					{Name: "registry.redhat.io/ubi8/ubi:latest"},
				},
				Helm: v2alpha1.Helm{
					Repositories: []v2alpha1.Repository{
						{
							URL:  "https://stefanprodan.github.io/podinfo",
							Name: "podinfo",
							Charts: []v2alpha1.Chart{
								{Name: "podinfo", Version: "5.0.0"},
							},
						},
					},
					Local: []v2alpha1.Chart{
						{Name: "podinfo", Path: "/test/podinfo-5.0.0.tar.gz"},
					},
				},
				BlockedImages: []v2alpha1.Image{
					{Name: "alpine"},
					{Name: "redis"},
				},
				Samples: []v2alpha1.SampleImages{
					{Image: v2alpha1.Image{Name: "ruby"}},
					{Image: v2alpha1.Image{Name: "python"}},
					{Image: v2alpha1.Image{Name: "nginx"}},
				},
			},
		},
	}

	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mirrorMock := mirrormock.NewMockMirrorInterface(mockCtrl)
	manifestMock := manifestmock.NewMockManifestInterface(mockCtrl)
	cincinnatiMock := releasemock.NewMockCincinnatiInterface(mockCtrl)

	// this test should cover over 80% M2D
	t.Run("Testing CreateGraphImage - Mirror to disk: should pass", func(t *testing.T) {
		builderMock := imgbuildermock.NewMockImageBuilderInterface(mockCtrl)

		builderMock.
			EXPECT().
			SaveImageLayoutToDir(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(layout.FromPath(common.TestFolder + "test-untar")).
			AnyTimes()

		builderMock.
			EXPECT().
			BuildAndPush(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return("sha256:12345", nil).
			AnyTimes()

		ex := &LocalStorageCollector{
			Log:              log,
			Mirror:           mirrorMock,
			Config:           cfgm2d,
			Manifest:         manifestMock,
			Opts:             m2dOpts,
			Cincinnati:       cincinnatiMock,
			LocalStorageFQDN: "localhost:9999",
			ImageBuilder:     builderMock,
		}

		// just to ensure we cover new.go
		_ = New(log, "nada", cfgm2d, m2dOpts, mirrorMock, manifestMock, cincinnatiMock, builderMock)

		_, err := ex.CreateGraphImage(ctx, graphURL)
		if err != nil {
			t.Fatalf("should not fail")
		}
	})

	t.Run("Testing CreateGraphImage - Mirror to disk: should fail", func(t *testing.T) {
		ex := &LocalStorageCollector{
			Log:              log,
			Mirror:           mirrorMock,
			Config:           cfgm2d,
			Manifest:         manifestMock,
			Opts:             m2dOpts,
			Cincinnati:       cincinnatiMock,
			LocalStorageFQDN: "localhost:9999",
			ImageBuilder:     imgbuildermock.NewMockImageBuilderInterface(mockCtrl),
		}

		_, err := ex.CreateGraphImage(ctx, "nada")
		if err == nil {
			t.Fatalf("should fail")
		}
	})

	t.Run("Testing CreateGraphImage - Mirror to disk: should fail", func(t *testing.T) {
		builderMock := imgbuildermock.NewMockImageBuilderInterface(mockCtrl)

		builderMock.
			EXPECT().
			BuildAndPush(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return("", errors.New("force-fail build and push")).
			AnyTimes()

		builderMock.
			EXPECT().
			SaveImageLayoutToDir(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(layout.Path(""), errors.New("force-fail save image layout")).
			AnyTimes()

		ex := &LocalStorageCollector{
			Log:              log,
			Mirror:           mirrorMock,
			Config:           cfgm2d,
			Manifest:         manifestMock,
			Opts:             m2dOpts,
			Cincinnati:       cincinnatiMock,
			LocalStorageFQDN: "localhost:9999",
			ImageBuilder:     builderMock,
		}

		_, err := ex.CreateGraphImage(ctx, graphURL)
		if err == nil {
			t.Fatalf("should fail")
		}
	})
}
