package release

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

// setup mocks
// we need to mock Manifest, Mirror, Cincinnati

type MockMirror struct {
	Fail bool
}

type MockManifest struct {
	Log               clog.PluggableLoggerInterface
	FailImageIndex    bool
	FailImageManifest bool
	FailExtract       bool
}

type MockCincinnati struct {
	Config v1alpha2.ImageSetConfiguration
	Opts   mirror.CopyOptions
	Client Client
	Fail   bool
}

func TestReleaseImageCollector(t *testing.T) {

	log := clog.New("trace")

	global := &mirror.GlobalOptions{
		TlsVerify:    false,
		SecurePolicy: false,
		WorkingDir:   "../../tests",
	}

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

	cincinnati := &MockCincinnati{Config: cfg, Opts: opts}
	ctx := context.Background()

	// this test should cover over 80%
	t.Run("Testing ReleaseImageCollector : should pass", func(t *testing.T) {
		manifest := &MockManifest{Log: log}
		ex := &Collector{
			Log:        log,
			Mirror:     &MockMirror{Fail: false},
			Config:     cfg,
			Manifest:   manifest,
			Opts:       opts,
			Cincinnati: cincinnati,
		}
		res, err := ex.ReleaseImageCollector(ctx)
		if err != nil {
			t.Fatalf("should not fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector : should fail mirror", func(t *testing.T) {
		os.RemoveAll("../../tests/hold-release/")
		os.RemoveAll("../../tests/release-images")
		manifest := &MockManifest{Log: log}
		ex := &Collector{
			Log:        log,
			Mirror:     &MockMirror{Fail: true},
			Config:     cfg,
			Manifest:   manifest,
			Opts:       opts,
			Cincinnati: cincinnati,
		}
		res, err := ex.ReleaseImageCollector(ctx)
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector : should fail image index", func(t *testing.T) {
		manifest := &MockManifest{Log: log, FailImageIndex: true}
		ex := &Collector{
			Log:        log,
			Mirror:     &MockMirror{Fail: false},
			Config:     cfg,
			Manifest:   manifest,
			Opts:       opts,
			Cincinnati: cincinnati,
		}
		res, err := ex.ReleaseImageCollector(ctx)
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector : should fail image manifest", func(t *testing.T) {
		manifest := &MockManifest{Log: log, FailImageManifest: true}
		ex := &Collector{
			Log:        log,
			Mirror:     &MockMirror{Fail: false},
			Config:     cfg,
			Manifest:   manifest,
			Opts:       opts,
			Cincinnati: cincinnati,
		}
		res, err := ex.ReleaseImageCollector(ctx)
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})

	t.Run("Testing ReleaseImageCollector : should fail extract", func(t *testing.T) {
		manifest := &MockManifest{Log: log, FailExtract: true}
		ex := &Collector{
			Log:        log,
			Mirror:     &MockMirror{Fail: false},
			Config:     cfg,
			Manifest:   manifest,
			Opts:       opts,
			Cincinnati: cincinnati,
		}
		res, err := ex.ReleaseImageCollector(ctx)
		if err == nil {
			t.Fatalf("should fail")
		}
		log.Debug("completed test related images %v ", res)
	})
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
	return nil, nil
}

func (o MockManifest) GetRelatedImagesFromCatalogByFilter(filePath, label string, op v1alpha2.Operator, mp map[string]v1alpha3.ISCPackage) (map[string][]v1alpha3.RelatedImage, error) {
	return nil, nil
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

func (o MockManifest) GetRelatedImagesFromCatalog(filePath, label string) (map[string][]v1alpha3.RelatedImage, error) {
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

func (o MockCincinnati) GetReleaseReferenceImages(ctx context.Context) []v1alpha3.CopyImageSchema {
	var res []v1alpha3.CopyImageSchema
	res = append(res, v1alpha3.CopyImageSchema{Source: "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64", Destination: "localhost:9999/ocp-release:4.13.10-x86_64"})
	return res
}

func (o MockCincinnati) NewOCPClient(uuid uuid.UUID) (Client, error) {
	if o.Fail {
		return o.Client, fmt.Errorf("forced cincinnati client error")
	}
	return o.Client, nil
}

func (o MockCincinnati) NewOKDClient(uuid uuid.UUID) (Client, error) {
	return o.Client, nil
}

func (o MockCincinnati) GenerateReleaseSignatures(context.Context, []v1alpha3.RelatedImage) {
	fmt.Println("test release signature")
}
