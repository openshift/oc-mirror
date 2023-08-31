//go:build unit_tests
// +build unit_tests

package release

import (
	"context"
	"os"
	"testing"

	"github.com/docker/docker/testutil/registry"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

func TestReleaseLocalStoredCollector(t *testing.T) {

	log := clog.New("trace")

	global := &mirror.GlobalOptions{
		TlsVerify:      false,
		InsecurePolicy: true,
		Dir:            "../../tests",
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	m2dOpts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Destination:         "file://test",
		Dev:                 false,
		Mode:                mirrorToDisk,
	}

	cfg := v1alpha2.ImageSetConfiguration{
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

	cincinnati := &Cincinnati{Config: cfg, Opts: m2dOpts}
	ctx := context.Background()

	// this test should cover over 80%
	t.Run("Testing ReleaseImageCollector : should pass", func(t *testing.T) {
		manifest := &Manifest{Log: log}
		ex := &LocalStorageCollector{
			Log:        log,
			Mirror:     &Mirror{Fail: false},
			Config:     cfg,
			Manifest:   manifest,
			Opts:       m2dOpts,
			Cincinnati: cincinnati,
		}
		mock, err := registry.NewMock(t)
		if err != nil {
			t.Fatalf("should not fail")
		}
		defer func() {
			mock.Close()
		}()

		ex.LocalStorageFQDN = mock.URL()
		res, err := ex.ReleaseImageCollector(ctx)
		if err != nil {
			t.Fatalf("should not fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector : should fail mirror", func(t *testing.T) {
		os.RemoveAll("../../tests/hold-release/")
		os.RemoveAll("../../tests/release-images")
		manifest := &Manifest{Log: log}
		ex := &Collector{
			Log:        log,
			Mirror:     &Mirror{Fail: true},
			Config:     cfg,
			Manifest:   manifest,
			Opts:       m2dOpts,
			Cincinnati: cincinnati,
		}
		res, err := ex.ReleaseImageCollector(ctx)
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector : should fail image index", func(t *testing.T) {
		manifest := &Manifest{Log: log, FailImageIndex: true}
		ex := &LocalStorageCollector{
			Log:        log,
			Mirror:     &Mirror{Fail: false},
			Config:     cfg,
			Manifest:   manifest,
			Opts:       m2dOpts,
			Cincinnati: cincinnati,
		}
		mock, err := registry.NewMock(t)
		if err != nil {
			t.Fatalf("should not fail")
		}
		defer func() {
			mock.Close()
		}()

		ex.LocalStorageFQDN = mock.URL()
		res, err := ex.ReleaseImageCollector(ctx)
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector : should fail image manifest", func(t *testing.T) {
		manifest := &Manifest{Log: log, FailImageManifest: true}
		ex := &LocalStorageCollector{
			Log:        log,
			Mirror:     &Mirror{Fail: false},
			Config:     cfg,
			Manifest:   manifest,
			Opts:       m2dOpts,
			Cincinnati: cincinnati,
		}
		mock, err := registry.NewMock(t)
		if err != nil {
			t.Fatalf("should not fail")
		}
		defer func() {
			mock.Close()
		}()

		ex.LocalStorageFQDN = mock.URL()
		res, err := ex.ReleaseImageCollector(ctx)
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector : should fail extract", func(t *testing.T) {
		manifest := &Manifest{Log: log, FailExtract: true}
		ex := &LocalStorageCollector{
			Log:        log,
			Mirror:     &Mirror{Fail: false},
			Config:     cfg,
			Manifest:   manifest,
			Opts:       m2dOpts,
			Cincinnati: cincinnati,
		}
		mock, err := registry.NewMock(t)
		if err != nil {
			t.Fatalf("should not fail")
		}
		defer func() {
			mock.Close()
		}()

		ex.LocalStorageFQDN = mock.URL()
		res, err := ex.ReleaseImageCollector(ctx)
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})
}
