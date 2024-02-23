package operator

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"path/filepath"
	"strings"
	"testing"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
	"github.com/otiai10/copy"
	//"github.com/stretchr/testify/assert"
)

type MockMirror struct {
	Fail bool
}

type MockManifest struct {
	Log               clog.PluggableLoggerInterface
	FailImageIndex    bool
	FailImageManifest bool
	FailExtract       bool
}

func TestOperatorLocalStoredCollector(t *testing.T) {
	log := clog.New("trace")

	tempDir := t.TempDir()
	defer os.RemoveAll(tempDir)

	// this test should cover over 80% M2D
	t.Run("Testing OperatorImageCollector - Mirror to disk: should pass", func(t *testing.T) {
		ctx := context.Background()
		manifest := &MockManifest{Log: log}

		ex := setupCollector_MirrorToDisk(tempDir, log, manifest)

		// ensure coverage in new.go
		_ = New(log, "working-dir", ex.Config, ex.Opts, ex.Mirror, manifest, "localhost:9999")

		res, err := ex.OperatorImageCollector(ctx)
		if err != nil {
			t.Fatalf("should not fail")
		}
		log.Debug("completed test related images %v ", res)

		// test with TargetTag
		ex.Config.Mirror.Operators[3].TargetTag = "v4.14"
		res, err = ex.OperatorImageCollector(ctx)
		if err != nil {
			t.Fatalf("should not fail")
		}
		log.Debug("completed test (with TargetTag set) related images %v ", res)

		// test with TargetCatalog
		ex.Config.Mirror.Operators[3].TargetTag = ""
		ex.Config.Mirror.Operators[3].TargetCatalog = "test-catalog:v4.14"
		res, err = ex.OperatorImageCollector(ctx)
		if err != nil {
			t.Fatalf("should not fail")
		}
		log.Debug("completed test (with TargetCatalog set) related images %v ", res)

	})

	t.Run("Testing OperatorImageCollector - Disk to mirror : should pass", func(t *testing.T) {

		os.RemoveAll("../../tests/hold-operator/")
		os.RemoveAll("../../tests/operator-images")
		os.RemoveAll("../../tests/tmp/")
		ex := setupCollector_DiskToMirror(tempDir, log)

		//copy tests/hold-test-fake to working-dir
		err := copy.Copy("../../tests/working-dir-fake/hold-operator/redhat-operator-index/v4.14", filepath.Join(ex.Opts.Global.WorkingDir, operatorImageExtractDir, "ocp-release/4.13.9-x86_64"))
		if err != nil {
			t.Fatalf("should not fail")
		}

		res, err := ex.OperatorImageCollector(context.Background())
		if err != nil {
			t.Fatalf("should not fail: %v", err)
		}
		if len(res) == 0 {
			t.Fatalf("should contain at least 1 image")
		}
		if !strings.Contains(res[0].Source, ex.LocalStorageFQDN) {
			t.Fatalf("source images should be from local storage")
		}
		log.Debug("completed test related images %v ", res)

		// test with TargetCatalog
		ex.Config.Mirror.Operators[1].TargetTag = ""
		ex.Config.Mirror.Operators[1].TargetCatalog = "test-catalog:v4.14"
		res, err = ex.OperatorImageCollector(context.Background())
		if err != nil {
			t.Fatalf("should not fail")
		}
		log.Debug("completed test (with TargetCatalog set) related images %v ", res)

	})
}

func setupCollector_DiskToMirror(tempDir string, log clog.PluggableLoggerInterface) *LocalStorageCollector {
	manifest := &MockManifest{Log: log}

	globalD2M := &mirror.GlobalOptions{
		TlsVerify:    false,
		SecurePolicy: false,
		WorkingDir:   tempDir + "/working-dir",
		From:         tempDir,
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOptsD2M := mirror.ImageSrcFlags(globalD2M, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOptsD2M := mirror.ImageDestFlags(globalD2M, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	d2mOpts := mirror.CopyOptions{
		Global:              globalD2M,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOptsD2M,
		DestImage:           destOptsD2M,
		RetryOpts:           retryOpts,
		Destination:         "docker://localhost:5000/test",
		Dev:                 false,
		Mode:                mirror.DiskToMirror,
	}

	cfgd2m := v1alpha2.ImageSetConfiguration{
		ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
			Mirror: v1alpha2.Mirror{
				Operators: []v1alpha2.Operator{
					{
						Catalog: "redhat-operator-index:v4.14",
					},
					{
						Catalog: "oci://../../tests/simple-test-bundle",
					},
				},
			},
		},
	}

	ex := &LocalStorageCollector{
		Log:              log,
		Mirror:           &MockMirror{Fail: false},
		Config:           cfgd2m,
		Manifest:         manifest,
		Opts:             d2mOpts,
		LocalStorageFQDN: "localhost:9999",
	}

	return ex
}

func setupCollector_MirrorToDisk(tempDir string, log clog.PluggableLoggerInterface, manifest *MockManifest) *LocalStorageCollector {

	globalM2D := &mirror.GlobalOptions{
		TlsVerify:    false,
		SecurePolicy: false,
		WorkingDir:   tempDir + "/working-dir",
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
					Graph: true,
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
					{
						Catalog: "oci://../../tests/simple-test-bundle",
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

	ex := &LocalStorageCollector{
		Log:              log,
		Mirror:           &MockMirror{Fail: false},
		Config:           cfgm2d,
		Manifest:         manifest,
		Opts:             m2dOpts,
		LocalStorageFQDN: "localhost:9999",
	}
	return ex
}

func (o MockMirror) Run(ctx context.Context, src, dest string, mode mirror.Mode, opts *mirror.CopyOptions, out bufio.Writer) error {
	if o.Fail {
		return fmt.Errorf("forced mirror run fail")
	}
	return nil
}

func (o MockMirror) Check(ctx context.Context, image string, opts *mirror.CopyOptions) (bool, error) {
	return true, nil
}

func (o MockManifest) GetOperatorConfig(file string) (*v1alpha3.OperatorConfigSchema, error) {
	var cfs *v1alpha3.OperatorConfigSchema
	// read the test config
	cfg, err := os.ReadFile("../../tests/operator-config.json")
	if err != nil {
		o.Log.Error("%v ", err)
	}
	err = json.Unmarshal(cfg, &cfs)
	if err != nil {
		o.Log.Error("%v ", err)
	}
	return cfs, nil
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
	if o.FailImageIndex {
		return &v1alpha3.OCISchema{}, fmt.Errorf("forced error image index")
	}
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
	if o.FailImageManifest {
		return &v1alpha3.OCISchema{}, fmt.Errorf("forced error image index")
	}

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
		{Name: "testA", Image: "sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testB", Image: "sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
	}
	return relatedImages, nil
}

func (o MockManifest) ExtractLayersOCI(filePath, toPath, label string, oci *v1alpha3.OCISchema) error {
	if o.FailExtract {
		return fmt.Errorf("forced extract oci fail")
	}
	return nil
}
