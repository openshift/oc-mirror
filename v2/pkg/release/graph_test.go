package release

import (
	"context"
	"fmt"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

type mockImageBuilder struct {
	Fail bool
}

func TestCreateGraphImage(t *testing.T) {

	log := clog.New("trace")
	globalM2D := &mirror.GlobalOptions{
		TlsVerify:    false,
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
	}

	cfgm2d := v1alpha2.ImageSetConfiguration{
		ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
			Mirror: v1alpha2.Mirror{
				Platform: v1alpha2.Platform{
					Graph: true,
					Channels: []v1alpha2.ReleaseChannel{
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
							Type: v1alpha2.TypeOKD,
						},
					},
				},
				Operators: []v1alpha2.Operator{
					{
						Catalog: "redhat-operators:v4.7",
						Full:    true,
					},
					{
						Catalog: "certified-operators:v4.7",
						Full:    true,
						IncludeConfig: v1alpha2.IncludeConfig{
							Packages: []v1alpha2.IncludePackage{
								{Name: "couchbase-operator"},
								{
									Name: "mongodb-operator",
									IncludeBundle: v1alpha2.IncludeBundle{
										MinVersion: "1.4.0",
									},
								},
								{
									Name: "crunchy-postgresql-operator",
									Channels: []v1alpha2.IncludeChannel{
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
				AdditionalImages: []v1alpha2.Image{
					{Name: "registry.redhat.io/ubi8/ubi:latest"},
				},
				Helm: v1alpha2.Helm{
					Repositories: []v1alpha2.Repository{
						{
							URL:  "https://stefanprodan.github.io/podinfo",
							Name: "podinfo",
							Charts: []v1alpha2.Chart{
								{Name: "podinfo", Version: "5.0.0"},
							},
						},
					},
					Local: []v1alpha2.Chart{
						{Name: "podinfo", Path: "/test/podinfo-5.0.0.tar.gz"},
					},
				},
				BlockedImages: []v1alpha2.Image{
					{Name: "alpine"},
					{Name: "redis"},
				},
				Samples: []v1alpha2.SampleImages{
					{Image: v1alpha2.Image{Name: "ruby"}},
					{Image: v1alpha2.Image{Name: "python"}},
					{Image: v1alpha2.Image{Name: "nginx"}},
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
		_ = New(log, "nada", cfgm2d, m2dOpts, &MockMirror{}, &MockManifest{}, cincinnati, "localhost:9999", &mockImageBuilder{})

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
	return layout.FromPath("../../tests/test-untar")
}

func (o mockImageBuilder) ProcessImageIndex(ctx context.Context, idx v1.ImageIndex, v2format *bool, cmd []string, targetRef string, layers ...v1.Layer) (v1.ImageIndex, error) {
	return nil, nil
}
