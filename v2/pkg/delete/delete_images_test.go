package delete

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	mirror "github.com/openshift/oc-mirror/v2/pkg/mirror"
	"github.com/stretchr/testify/assert"
)

// TestAllDeleteImages
func TestAllDeleteImages(t *testing.T) {
	log := clog.New("trace")

	global := &mirror.GlobalOptions{
		TlsVerify:    false,
		SecurePolicy: false,
		Quiet:        false,
		WorkingDir:   "../../tests/",
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
		Destination:         "oci:test",
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
	}

	isc := v1alpha2.ImageSetConfiguration{
		ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
			Mirror: v1alpha2.Mirror{
				Operators: []v1alpha2.Operator{
					{
						Catalog: "redhat-operator-index:v4.14",
						IncludeConfig: v1alpha2.IncludeConfig{
							Packages: []v1alpha2.IncludePackage{
								{Name: "node-observability-operator"},
							},
						},
					},
				},
				AdditionalImages: []v1alpha2.Image{
					{Name: "test.registry.io/test-image@sha256:c4b775cbe8eec55de2c163919c6008599e2aebe789ed93ada9a307e800e3f1e2"},
				},
			},
		},
	}

	di := New(log, opts, &mockBatch{}, &mockBlobs{}, isc, &mockManifest{}, "/tmp", "localhost:8888")

	t.Run("Testing ReadDeleteData : should pass", func(t *testing.T) {
		opts.Global.WorkingDir = "../../tests"
		data, err := di.ReadDeleteMetaData()
		if err != nil {
			t.Fatal("should not fail")
		}
		assert.Equal(t, "docker://localhost:55000/openshift-release-dev/ocp-v4.0-art-dev@sha256:c4b775cbe8eec55de2c163919c6008599e2aebe789ed93ada9a307e800e3f1e2", data.Items[0].ImageReference)
	})

	t.Run("Testing CollectReleaseImages : should pass", func(t *testing.T) {
		data, err := di.CollectReleaseImages("../../tests/working-dir-fake/hold-release/ocp-release/4.14.1-x86_64")
		if err != nil {
			t.Fatal("should not fail")
		}
		assert.Equal(t, len(data), 183)
		assert.Equal(t, "docker://localhost:8888/openshift-release-dev/ocp-v4.0-art-dev@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e4182331e", data[0].Destination)
	})

	t.Run("Testing CollectOperatorImages : should pass", func(t *testing.T) {
		opts.Global.WorkingDir = "../../tests/working-dir-fake"
		data, err := di.CollectOperatorImages()
		if err != nil {
			t.Fatal("should not fail")
		}
		assert.Equal(t, "docker://localhost:8888/test-image:v1.0.0", data[0].Destination)
	})

	t.Run("Testing CollectAdditionalImages : should pass", func(t *testing.T) {
		data, err := di.CollectAdditionalImages()
		if err != nil {
			t.Fatal("should not fail")
		}
		assert.Equal(t, "docker://localhost:8888/test-image@sha256:c4b775cbe8eec55de2c163919c6008599e2aebe789ed93ada9a307e800e3f1e2", data[0].Destination)
	})

	t.Run("Testing DeleteRegistryImages : should pass", func(t *testing.T) {
		opts.Global.WorkingDir = "../../tests"
		imgs, err := di.ReadDeleteMetaData()
		if err != nil {
			t.Fatal("should not fail")
		}
		err = di.DeleteRegistryImages(imgs)
		if err != nil {
			t.Fatal("should not fail")
		}
	})

	t.Run("Testing DeleteCacheBlobs : should pass", func(t *testing.T) {
		testFolder := t.TempDir()
		defer os.RemoveAll(testFolder)
		opts.Global.WorkingDir = "../../tests"
		deleteDI := New(log, opts, &mockBatch{}, &mockBlobs{}, v1alpha2.ImageSetConfiguration{}, &mockManifest{}, "/tmp", "localhost:8888")
		imgs, err := di.ReadDeleteMetaData()
		if err != nil {
			t.Fatal("should not fail")
		}
		err = deleteDI.DeleteCacheBlobs(imgs)
		if err != nil {
			t.Fatal("should not fail")
		}
	})

}

// mockBatch
type mockBatch struct {
	Fail bool
}

// mockBlobs
type mockBlobs struct {
	Fail bool
}

type mockManifest struct{}

func (o *mockBatch) Worker(ctx context.Context, images []v1alpha3.CopyImageSchema, opts mirror.CopyOptions) error {
	if o.Fail {
		return fmt.Errorf("forced error")
	}
	return nil
}

func (o *mockBlobs) GatherBlobs(ctx context.Context, image string) (map[string]string, error) {
	res := map[string]string{"sha256:95ad8395795ee0460baf05458f669d3b865535f213f015519ef9a221a6a08280": ""}
	if o.Fail {
		return nil, fmt.Errorf("forced error")
	}
	return res, nil
}

func (o mockManifest) GetImageIndex(dir string) (*v1alpha3.OCISchema, error) {
	return &v1alpha3.OCISchema{}, nil
}

func (o mockManifest) GetImageManifest(file string) (*v1alpha3.OCISchema, error) {
	return &v1alpha3.OCISchema{}, nil
}

func (o mockManifest) GetOperatorConfig(file string) (*v1alpha3.OperatorConfigSchema, error) {
	return &v1alpha3.OperatorConfigSchema{}, nil
}

func (o mockManifest) GetCatalog(filePath string) (manifest.OperatorCatalog, error) {
	return manifest.OperatorCatalog{}, nil
}

func (o mockManifest) GetRelatedImagesFromCatalog(operatorCatalog manifest.OperatorCatalog, ctlgInIsc v1alpha2.Operator) (map[string][]v1alpha3.RelatedImage, error) {
	res := map[string][]v1alpha3.RelatedImage{}
	ri := []v1alpha3.RelatedImage{
		{
			Name:  "node-observability-agent",
			Image: "test.registry.io/test-image:v1.0.0",
		},
		{
			Name:  "node-observability-controller",
			Image: "test.registry.io/test-image-controller:v1.0.0",
		},
	}
	res["node-observability-operator"] = ri
	return res, nil
}

func (o mockManifest) ExtractLayersOCI(filePath, toPath, label string, oci *v1alpha3.OCISchema) error {
	return nil
}

func (o mockManifest) GetReleaseSchema(filePath string) ([]v1alpha3.RelatedImage, error) {
	return []v1alpha3.RelatedImage{}, nil
}

func (o mockManifest) ConvertIndexToSingleManifest(dir string, oci *v1alpha3.OCISchema) error {
	return nil
}
