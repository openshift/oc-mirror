package batch

import (
	"context"
	"os"
	"testing"

	"github.com/containers/image/v5/types"
	"github.com/openshift/oc-mirror/v2/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

func TestWorker(t *testing.T) {

	log := clog.New("trace")

	global := &mirror.GlobalOptions{SecurePolicy: false, Quiet: false}

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
		Destination:         "oci:test",
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
	}
	tempDir := t.TempDir()
	defer os.RemoveAll(tempDir)

	w := New(log, tempDir, &Mirror{}, &Manifest{})

	// this is a facade to get code coverage up
	t.Run("Testing Worker : should pass", func(t *testing.T) {
		relatedImages := []v2alpha1.CopyImageSchema{
			{Source: "docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
			{Source: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
			{Source: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
			{Source: "docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
			{Source: "docker://registry/name/namespace/sometestimage-e@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
			{Source: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
			{Source: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
			{Source: "docker://registry/name/namespace/sometestimage-h@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
			{Source: "docker://registry/name/namespace/sometestimage-i@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
		}
		err := w.Worker(context.Background(), v2alpha1.CollectorSchema{AllImages: relatedImages}, opts)
		if err != nil {
			t.Fatal("should pass")
		}
	})
}

// mocks

type Mirror struct{}
type Manifest struct{}

func (o Mirror) Run(ctx context.Context, src, dest string, mode mirror.Mode, opts *mirror.CopyOptions) error {
	return nil
}

func (o Mirror) Check(ctx context.Context, image string, opts *mirror.CopyOptions) (bool, error) {
	return true, nil
}

func (o Manifest) GetOperatorConfig(file string) (*v2alpha1.OperatorConfigSchema, error) {
	opcl := v2alpha1.OperatorLabels{OperatorsOperatorframeworkIoIndexConfigsV1: "/configs"}
	opc := v2alpha1.OperatorConfig{Labels: opcl}
	ocs := &v2alpha1.OperatorConfigSchema{Config: opc}
	return ocs, nil
}

func (o Manifest) GetReleaseSchema(filePath string) ([]v2alpha1.RelatedImage, error) {
	relatedImages := []v2alpha1.RelatedImage{
		{Name: "testA", Image: "registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testB", Image: "registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testC", Image: "registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testD", Image: "registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
	}
	return relatedImages, nil
}

func (o Manifest) GetImageIndex(name string) (*v2alpha1.OCISchema, error) {
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

func (o Manifest) GetImageManifest(name string) (*v2alpha1.OCISchema, error) {
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

func (o Manifest) GetCatalog(filePath string) (manifest.OperatorCatalog, error) {
	return manifest.OperatorCatalog{}, nil
}

func (o Manifest) GetRelatedImagesFromCatalog(operatorCatalog manifest.OperatorCatalog, ctlgInIsc v2alpha1.Operator) (map[string][]v2alpha1.RelatedImage, error) {
	relatedImages := make(map[string][]v2alpha1.RelatedImage)
	relatedImages["abc"] = []v2alpha1.RelatedImage{
		{Name: "testA", Image: "sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
		{Name: "testB", Image: "sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"},
	}
	return relatedImages, nil
}

func (o Manifest) ExtractLayersOCI(filePath, toPath, label string, oci *v2alpha1.OCISchema) error {
	return nil
}

func (o Manifest) ExtractLayers(filePath, name, label string) error {
	return nil
}

func (o Manifest) ConvertIndexToSingleManifest(dir string, oci *v2alpha1.OCISchema) error {
	return nil
}

func (o Manifest) GetDigest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string) (string, error) {
	return "", nil
}
