package delete

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/containers/image/v5/types"
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/common"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	mirror "github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/stretchr/testify/assert"
)

// TestAllDeleteImages
func TestAllDeleteImages(t *testing.T) {
	log := clog.New("trace")

	global := &mirror.GlobalOptions{
		SecurePolicy:      false,
		Quiet:             false,
		WorkingDir:        common.TestFolder,
		DeleteDestination: "docker://myregistry",
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
		Destination:         "docker://myregistry",
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
		LocalStorageFQDN:    "localhost:8888",
	}

	isc := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Operators: []v2alpha1.Operator{
					{
						Catalog: "redhat-operator-index:v4.14",
						IncludeConfig: v2alpha1.IncludeConfig{
							Packages: []v2alpha1.IncludePackage{
								{Name: "node-observability-operator"},
							},
						},
					},
				},
				AdditionalImages: []v2alpha1.Image{
					{Name: "test.registry.io/test-image@sha256:c4b775cbe8eec55de2c163919c6008599e2aebe789ed93ada9a307e800e3f1e2"},
				},
			},
		},
	}

	di := New(log, opts, &mockBatch{}, &mockBlobs{}, isc, &mockManifest{}, "/tmp")

	t.Run("Testing ReadDeleteData : should pass", func(t *testing.T) {
		opts.Global.WorkingDir = common.TestFolder
		data, err := di.ReadDeleteMetaData()
		if err != nil {
			t.Fatal("should not fail")
		}
		assert.Equal(t, "docker://localhost:55000/openshift-release-dev/ocp-v4.0-art-dev@sha256:c4b775cbe8eec55de2c163919c6008599e2aebe789ed93ada9a307e800e3f1e2", data.Items[0].ImageReference)
	})

	t.Run("Testing DeleteRegistryImages : should pass", func(t *testing.T) {
		opts.Global.WorkingDir = common.TestFolder
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
		opts.Global.WorkingDir = common.TestFolder
		opts.Global.ForceCacheDelete = true
		deleteDI := New(log, opts, &mockBatch{}, &mockBlobs{}, v2alpha1.ImageSetConfiguration{}, &mockManifest{}, "/tmp")
		imgs, err := di.ReadDeleteMetaData()
		if err != nil {
			t.Fatal("should not fail")
		}

		err = deleteDI.DeleteRegistryImages(imgs)
		if err != nil {
			t.Fatal("should not fail")
		}
	})
}

// TestWriteMetaData
func TestWriteMetaData(t *testing.T) {
	log := clog.New("trace")

	tempDir := t.TempDir()
	defer os.RemoveAll(tempDir)

	global := &mirror.GlobalOptions{
		SecurePolicy: false,
		Quiet:        false,
		WorkingDir:   tempDir,
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
		LocalStorageFQDN:    "localhost:8888",
	}

	cfg := v2alpha1.ImageSetConfiguration{}
	di := New(log, opts, &mockBatch{}, &mockBlobs{}, cfg, &mockManifest{}, "/tmp")

	t.Run("Testing ReadDeleteData : should pass", func(t *testing.T) {
		cpImages := []v2alpha1.CopyImageSchema{
			{
				Source:      "docker://localhost:55000/openshift-release-dev/ocp-v4.0-art-dev@sha256:c4b775cbe8eec55de2c163919c6008599e2aebe789ed93ada9a307e800e3f1e2",
				Destination: "docker://localhost:55000/openshift-release-dev/ocp-v4.0-art-dev@sha256:c4b775cbe8eec55de2c163919c6008599e2aebe789ed93ada9a307e800e3f1e2",
				Origin:      "test",
			},
		}
		err := di.WriteDeleteMetaData(cpImages)
		if err != nil {
			t.Fatalf("should not fail %v", err)
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

func (o mockBatch) Worker(ctx context.Context, collectorSchema v2alpha1.CollectorSchema, opts mirror.CopyOptions) (v2alpha1.CollectorSchema, error) {
	copiedImages := v2alpha1.CollectorSchema{
		AllImages:             []v2alpha1.CopyImageSchema{},
		TotalReleaseImages:    0,
		TotalOperatorImages:   0,
		TotalAdditionalImages: 0,
	}
	if o.Fail {
		return copiedImages, fmt.Errorf("forced error")
	}
	return collectorSchema, nil
}

func (o *mockBlobs) GatherBlobs(ctx context.Context, image string) (map[string]string, error) {
	res := map[string]string{"sha256:95ad8395795ee0460baf05458f669d3b865535f213f015519ef9a221a6a08280": ""}
	if o.Fail {
		return nil, fmt.Errorf("forced error")
	}
	return res, nil
}

func (o mockManifest) GetImageIndex(dir string) (*v2alpha1.OCISchema, error) {
	return &v2alpha1.OCISchema{}, nil
}

func (o mockManifest) GetImageManifest(file string) (*v2alpha1.OCISchema, error) {
	return &v2alpha1.OCISchema{}, nil
}

func (o mockManifest) GetOperatorConfig(file string) (*v2alpha1.OperatorConfigSchema, error) {
	return &v2alpha1.OperatorConfigSchema{}, nil
}

func (o mockManifest) GetCatalog(filePath string) (manifest.OperatorCatalog, error) {
	return manifest.OperatorCatalog{}, nil
}

func (o mockManifest) GetRelatedImagesFromCatalog(operatorCatalog manifest.OperatorCatalog, ctlgInIsc v2alpha1.Operator, copyImageSchemaMap *v2alpha1.CopyImageSchemaMap) (map[string][]v2alpha1.RelatedImage, error) {
	res := map[string][]v2alpha1.RelatedImage{}
	ri := []v2alpha1.RelatedImage{
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

func (o mockManifest) ExtractLayersOCI(filePath, toPath, label string, oci *v2alpha1.OCISchema) error {
	return nil
}

func (o mockManifest) GetReleaseSchema(filePath string) ([]v2alpha1.RelatedImage, error) {
	return []v2alpha1.RelatedImage{}, nil
}

func (o mockManifest) ConvertIndexToSingleManifest(dir string, oci *v2alpha1.OCISchema) error {
	return nil
}

func (o mockManifest) GetDigest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string) (string, error) {
	return "", nil
}
