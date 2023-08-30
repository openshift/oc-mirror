package release

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otiai10/copy"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

func TestReleaseLocalStoredCollector(t *testing.T) {

	log := clog.New("trace")
	globalM2D := &mirror.GlobalOptions{
		TlsVerify:      false,
		InsecurePolicy: true,
		Dir:            t.TempDir(),
	}

	tmpDir := t.TempDir()
	globalD2M := &mirror.GlobalOptions{
		TlsVerify:      false,
		InsecurePolicy: true,
		Dir:            tmpDir + "/working-dir",
		From:           tmpDir,
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOptsD2M := mirror.ImageSrcFlags(globalD2M, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOptsD2M := mirror.ImageDestFlags(globalD2M, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
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
		Mode:                mirrorToDisk,
	}

	d2mOpts := mirror.CopyOptions{
		Global:              globalD2M,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOptsD2M,
		DestImage:           destOptsD2M,
		RetryOpts:           retryOpts,
		Destination:         "docker://localhost:5000/test",
		Dev:                 false,
		Mode:                diskToMirror,
	}

	cfgd2m := v1alpha2.ImageSetConfiguration{
		ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
			Mirror: v1alpha2.Mirror{
				Platform: v1alpha2.Platform{
					Architectures: []string{"amd64"},
					Channels: []v1alpha2.ReleaseChannel{
						{
							Name:       "stable-4.13",
							MinVersion: "4.13.9",
							MaxVersion: "4.13.10",
						},
					},
				},
			},
		},
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

	cincinnati := &Cincinnati{Config: cfgm2d, Opts: m2dOpts}
	ctx := context.Background()

	// this test should cover over 80% M2D
	t.Run("Testing ReleaseImageCollector - Mirror to disk: should pass", func(t *testing.T) {
		manifest := &Manifest{Log: log}
		ex := &LocalStorageCollector{
			Log:              log,
			Mirror:           &Mirror{Fail: false},
			Config:           cfgm2d,
			Manifest:         manifest,
			Opts:             m2dOpts,
			Cincinnati:       cincinnati,
			LocalStorageFQDN: "localhost:9999",
		}

		res, err := ex.ReleaseImageCollector(ctx)
		if err != nil {
			t.Fatalf("should not fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector - Disk to mirror : should pass", func(t *testing.T) {
		os.RemoveAll("../../tests/hold-release/")
		os.RemoveAll("../../tests/release-images")
		os.RemoveAll("../../tests/tmp/")
		//copy tests/hold-test-fake to working-dir
		err := copy.Copy("../../tests/hold-test-fake", filepath.Join(d2mOpts.Global.Dir, releaseImageExtractDir, "ocp-release/4.13.9-x86_64"))
		if err != nil {
			t.Fatalf("should not fail")
		}
		//copy tests/release-filters-fake to working-dir
		err = copy.Copy("../../tests/release-filters-fake/d5f8153de54b0327ad20d24d4dbba7a6", filepath.Join(d2mOpts.Global.Dir, releaseFiltersDir, "d5f8153de54b0327ad20d24d4dbba7a6"))
		if err != nil {
			t.Fatalf("should not fail")
		}
		//copy tests/release-filters-fake to working-dir
		err = copy.Copy("../../tests/release-filters-fake/d5f8153de54b0327ad20d24d4dbba7a6", filepath.Join(d2mOpts.Global.Dir, releaseFiltersDir, "d5f8153de54b0327ad20d24d4dbba7a6"))
		if err != nil {
			t.Fatalf("should not fail")
		}
		manifest := &Manifest{Log: log}
		ex := &LocalStorageCollector{
			Log:              log,
			Mirror:           &Mirror{Fail: false},
			Config:           cfgd2m,
			Manifest:         manifest,
			Opts:             d2mOpts,
			Cincinnati:       cincinnati,
			LocalStorageFQDN: "localhost:9999",
		}

		res, err := ex.ReleaseImageCollector(ctx)
		if err != nil {
			t.Fatalf("should not fail")
		}
		if len(res) == 0 {
			t.Fatalf("should contain at least 1 image")
		}
		if !strings.Contains(res[0].Source, ex.LocalStorageFQDN) {
			t.Fatalf("source images should be from local storage")
		}
		log.Debug("completed test related images %v ", res)
	})
	t.Run("Testing ReleaseImageCollector : should fail mirror", func(t *testing.T) {
		os.RemoveAll(m2dOpts.Global.Dir)
		manifest := &Manifest{Log: log}
		ex := &LocalStorageCollector{
			Log:              log,
			Mirror:           &Mirror{Fail: true},
			Config:           cfgm2d,
			Manifest:         manifest,
			Opts:             m2dOpts,
			Cincinnati:       cincinnati,
			LocalStorageFQDN: "localhost:9999",
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
			Log:              log,
			Mirror:           &Mirror{Fail: false},
			Config:           cfgm2d,
			Manifest:         manifest,
			Opts:             m2dOpts,
			Cincinnati:       cincinnati,
			LocalStorageFQDN: "localhost:9999",
		}

		res, err := ex.ReleaseImageCollector(ctx)
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector : should fail image manifest", func(t *testing.T) {
		manifest := &Manifest{Log: log, FailImageManifest: true}
		ex := &LocalStorageCollector{
			Log:              log,
			Mirror:           &Mirror{Fail: false},
			Config:           cfgm2d,
			Manifest:         manifest,
			Opts:             m2dOpts,
			Cincinnati:       cincinnati,
			LocalStorageFQDN: "localhost:9999",
		}

		res, err := ex.ReleaseImageCollector(ctx)
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector : should fail extract", func(t *testing.T) {
		manifest := &Manifest{Log: log, FailExtract: true}
		ex := &LocalStorageCollector{
			Log:              log,
			Mirror:           &Mirror{Fail: false},
			Config:           cfgm2d,
			Manifest:         manifest,
			Opts:             m2dOpts,
			Cincinnati:       cincinnati,
			LocalStorageFQDN: "localhost:9999",
		}

		res, err := ex.ReleaseImageCollector(ctx)
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})

}
