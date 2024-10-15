package release

import (
	"context"
	"fmt"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/common"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type mockImageBuilder struct {
	Fail bool
}

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

	cincinnati := &MockCincinnati{Config: cfgm2d, Opts: m2dOpts}

	ctx := context.Background()

	// this test should cover over 80% M2D
	t.Run("Testing CreateGraphImage - Mirror to disk: should pass", func(t *testing.T) {
		manifest := &MockManifest{Log: log}
		ex := &LocalStorageCollector{
			Log:              log,
			Mirror:           &MockMirror{Fail: false},
			Config:           cfgm2d,
			Manifest:         manifest,
			Opts:             m2dOpts,
			Cincinnati:       cincinnati,
			LocalStorageFQDN: "localhost:9999",
			ImageBuilder:     &mockImageBuilder{},
		}

		// just to ensure we cover new.go
		_ = New(log, "nada", cfgm2d, m2dOpts, &MockMirror{}, &MockManifest{}, cincinnati, &mockImageBuilder{})

		_, err := ex.CreateGraphImage(ctx, graphURL)
		if err != nil {
			t.Fatalf("should not fail")
		}

	})

	t.Run("Testing CreateGraphImage - Mirror to disk: should fail", func(t *testing.T) {
		manifest := &MockManifest{Log: log}
		ex := &LocalStorageCollector{
			Log:              log,
			Mirror:           &MockMirror{Fail: false},
			Config:           cfgm2d,
			Manifest:         manifest,
			Opts:             m2dOpts,
			Cincinnati:       cincinnati,
			LocalStorageFQDN: "localhost:9999",
			ImageBuilder:     &mockImageBuilder{},
		}

		_, err := ex.CreateGraphImage(ctx, "nada")
		if err == nil {
			t.Fatalf("should fail")
		}

	})

	t.Run("Testing CreateGraphImage - Mirror to disk: should fail", func(t *testing.T) {
		manifest := &MockManifest{Log: log}
		ex := &LocalStorageCollector{
			Log:              log,
			Mirror:           &MockMirror{Fail: false},
			Config:           cfgm2d,
			Manifest:         manifest,
			Opts:             m2dOpts,
			Cincinnati:       cincinnati,
			LocalStorageFQDN: "localhost:9999",
			ImageBuilder:     &mockImageBuilder{Fail: true},
		}

		_, err := ex.CreateGraphImage(ctx, graphURL)
		if err == nil {
			t.Fatalf("should fail")
		}

	})

}

func (o mockImageBuilder) BuildAndPush(ctx context.Context, targetRef string, layoutPath layout.Path, cmd []string, layers ...v1.Layer) error {
	if o.Fail {
		return fmt.Errorf("forced error")
	}
	return nil
}

func (o mockImageBuilder) SaveImageLayoutToDir(ctx context.Context, imgRef string, layoutDir string) (layout.Path, error) {
	if o.Fail {
		return layout.Path(""), fmt.Errorf("forced error")
	}
	return layout.FromPath(common.TestFolder + "test-untar")
}

func (o mockImageBuilder) ProcessImageIndex(ctx context.Context, idx v1.ImageIndex, v2format *bool, cmd []string, targetRef string, layers ...v1.Layer) (v1.ImageIndex, error) {
	return nil, nil
}

func (o mockImageBuilder) RebuildCatalogs(ctx context.Context, collectorSchema v2alpha1.CollectorSchema) ([]v2alpha1.CopyImageSchema, []v2alpha1.Image, error) {
	return []v2alpha1.CopyImageSchema{}, []v2alpha1.Image{}, nil
}
