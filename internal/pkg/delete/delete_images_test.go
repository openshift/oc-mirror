package delete

import (
	"context"
	"fmt"
	"testing"

	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/opencontainers/go-digest"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"go.podman.io/image/v5/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	mirror "github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type mockSignatureHandler struct{}

func (m *mockSignatureHandler) GetSignatureTag(ctx context.Context, imgRef string) ([]string, error) {
	return []string{"sha256-c8636a92b5665988f030ed0948225276fea7428f2fe1f227142c988dc409a515.sig"}, nil
}

// TestAllDeleteImages
func TestAllDeleteImages(t *testing.T) {
	log := clog.New("trace")

	global := &mirror.GlobalOptions{
		SecurePolicy:      false,
		Quiet:             false,
		WorkingDir:        consts.TestFolder,
		DeleteDestination: consts.DockerProtocol + "localhost:5000/myregistry",
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
		Destination:         consts.DockerProtocol + "myregistry",
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
				AdditionalImages: []v2alpha1.AdditionalImage{
					{Name: "test.registry.io/test-image@sha256:c4b775cbe8eec55de2c163919c6008599e2aebe789ed93ada9a307e800e3f1e2"},
				},
			},
		},
	}

	di := New(log, opts, &mockBatch{}, &mockBlobs{}, isc, &mockManifest{}, "/tmp", &mockSignatureHandler{})

	t.Run("Testing ReadDeleteData : should pass", func(t *testing.T) {
		opts.Global.WorkingDir = consts.TestFolder
		data, err := di.ReadDeleteMetaData()
		if err != nil {
			t.Fatal("should not fail")
		}
		assert.Equal(t, consts.DockerProtocol+"localhost:5000/myregistry/openshift/release:4.15.12-x86_64-agent-installer-api-server", data.Items[0].ImageReference)
	})

	t.Run("Testing DeleteRegistryImages : should pass", func(t *testing.T) {
		opts.Global.WorkingDir = consts.TestFolder
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
		opts.Global.WorkingDir = consts.TestFolder
		opts.Global.ForceCacheDelete = true
		deleteDI := New(log, opts, &mockBatch{}, &mockBlobs{}, v2alpha1.ImageSetConfiguration{}, &mockManifest{}, "/tmp", &mockSignatureHandler{})
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
	di := New(log, opts, &mockBatch{}, &mockBlobs{}, cfg, &mockManifest{}, "/tmp", &mockSignatureHandler{})

	t.Run("Testing ReadDeleteData : should pass", func(t *testing.T) {
		cpImages := []v2alpha1.CopyImageSchema{
			{
				Source:      consts.DockerProtocol + "localhost:55000/openshift-release-dev/ocp-v4.0-art-dev@sha256:c4b775cbe8eec55de2c163919c6008599e2aebe789ed93ada9a307e800e3f1e2",
				Destination: consts.DockerProtocol + "localhost:55000/openshift-release-dev/ocp-v4.0-art-dev@sha256:c4b775cbe8eec55de2c163919c6008599e2aebe789ed93ada9a307e800e3f1e2",
				Origin:      "test",
			},
		}
		err := di.WriteDeleteMetaData(context.Background(), cpImages)
		if err != nil {
			t.Fatalf("should not fail %v", err)
		}
	})
}

func TestSigDeleteItems(t *testing.T) {
	tempDir := t.TempDir()

	global := &mirror.GlobalOptions{
		SecurePolicy: false,
		Quiet:        false,
		WorkingDir:   tempDir,
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")

	tests := []struct {
		name     string
		img      v2alpha1.CopyImageSchema
		opts     mirror.CopyOptions
		expected []v2alpha1.DeleteItem
	}{
		{
			name: "SignaturesDisabled",
			img: v2alpha1.CopyImageSchema{
				Source:      "registry.example.com/ns/img@sha256:c8636a92b5665988f030ed0948225276fea7428f2fe1f227142c988dc409a515",
				Origin:      "registry.example.com/ns/img@sha256:c8636a92b5665988f030ed0948225276fea7428f2fe1f227142c988dc409a515",
				Destination: "mirror.example.com/ns/img@sha256:c8636a92b5665988f030ed0948225276fea7428f2fe1f227142c988dc409a515",
				Type:        v2alpha1.TypeGeneric,
			},
			opts: mirror.CopyOptions{
				Global: &mirror.GlobalOptions{
					DeleteSignatures: false,
				},
			},
			expected: []v2alpha1.DeleteItem{},
		},
		{
			name: "ValidSignatureTag",
			img: v2alpha1.CopyImageSchema{
				Source:      "registry.example.com/ns/img@sha256:c8636a92b5665988f030ed0948225276fea7428f2fe1f227142c988dc409a515",
				Origin:      "registry.example.com/ns/img@sha256:c8636a92b5665988f030ed0948225276fea7428f2fe1f227142c988dc409a515",
				Destination: "mirror.example.com/ns/img@sha256:c8636a92b5665988f030ed0948225276fea7428f2fe1f227142c988dc409a515",
				Type:        v2alpha1.TypeGeneric,
			},
			opts: mirror.CopyOptions{
				Global: &mirror.GlobalOptions{
					DeleteSignatures: true,
				},
				SrcImage: srcOpts,
			},
			expected: []v2alpha1.DeleteItem{
				{
					ImageName:      "registry.example.com/ns/img:sha256-c8636a92b5665988f030ed0948225276fea7428f2fe1f227142c988dc409a515.sig",
					ImageReference: consts.DockerProtocol + "mirror.example.com/ns/img:sha256-c8636a92b5665988f030ed0948225276fea7428f2fe1f227142c988dc409a515.sig",
					Type:           v2alpha1.TypeGeneric,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := DeleteImages{
				Opts:       tt.opts,
				SigHandler: &mockSignatureHandler{},
			}
			items := d.sigDeleteItems(context.Background(), tt.img)
			assert.Equal(t, tt.expected, items)
		})
	}
}

func TestGetSignatureTagWithoutCache(t *testing.T) {
	tests := []struct {
		name     string
		img      v2alpha1.CopyImageSchema
		expected *v2alpha1.DeleteItem
	}{
		{
			name: "ValidDigest",
			img: v2alpha1.CopyImageSchema{
				Origin:      "registry.example.com/ns/img@sha256:c8636a92b5665988f030ed0948225276fea7428f2fe1f227142c988dc409a515",
				Destination: "mirror.example.com/ns/img@sha256:c8636a92b5665988f030ed0948225276fea7428f2fe1f227142c988dc409a515",
				Type:        v2alpha1.TypeGeneric,
			},
			expected: &v2alpha1.DeleteItem{
				ImageName:      "registry.example.com/ns/img:sha256-c8636a92b5665988f030ed0948225276fea7428f2fe1f227142c988dc409a515.sig",
				ImageReference: consts.DockerProtocol + "mirror.example.com/ns/img:sha256-c8636a92b5665988f030ed0948225276fea7428f2fe1f227142c988dc409a515.sig",
				Type:           v2alpha1.TypeGeneric,
			},
		},
		{
			name: "NoDigest",
			img: v2alpha1.CopyImageSchema{
				Origin:      "registry.example.com/ns/img:latest",
				Destination: "mirror.example.com/ns/img:latest",
				Type:        v2alpha1.TypeGeneric,
			},
			expected: nil,
		},
		{
			name: "InvalidReference",
			img: v2alpha1.CopyImageSchema{
				Origin:      "invalid reference",
				Destination: "invalid reference",
				Type:        v2alpha1.TypeGeneric,
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := DeleteImages{}
			item := d.getSignatureTagWithoutCache(tt.img)
			assert.Equal(t, tt.expected, item)
		})
	}
}

func TestSigDeleteItem(t *testing.T) {
	tests := []struct {
		name     string
		img      v2alpha1.CopyImageSchema
		sig      string
		expected *v2alpha1.DeleteItem
	}{
		{
			name: "ValidSignature",
			img: v2alpha1.CopyImageSchema{
				Origin:      "registry.example.com/ns/img:latest",
				Destination: "mirror.example.com/ns/img:latest",
				Type:        v2alpha1.TypeGeneric,
			},
			sig: "sha256-abc123.sig",
			expected: &v2alpha1.DeleteItem{
				ImageName:      "registry.example.com/ns/img:sha256-abc123.sig",
				ImageReference: consts.DockerProtocol + "mirror.example.com/ns/img:sha256-abc123.sig",
				Type:           v2alpha1.TypeGeneric,
			},
		},
		{
			name: "EmptySignature",
			img: v2alpha1.CopyImageSchema{
				Origin:      "registry.example.com/ns/img:latest",
				Destination: "mirror.example.com/ns/img:latest",
				Type:        v2alpha1.TypeGeneric,
			},
			sig:      "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := DeleteImages{}
			item := d.sigDeleteItem(tt.img, tt.sig)
			assert.Equal(t, tt.expected, item)
		})
	}
}

// TestDeleteImagesWithTargetRepoAndTag tests deletion of images that use targetRepo and targetTag
func TestDeleteImagesWithTargetRepoAndTag(t *testing.T) {
	log := clog.New("trace")

	tempDir := t.TempDir()

	global := &mirror.GlobalOptions{
		SecurePolicy:      false,
		Quiet:             false,
		WorkingDir:        tempDir,
		DeleteDestination: "docker://localhost:5000/myregistry",
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

	cfg := v2alpha1.ImageSetConfiguration{}
	di := New(log, opts, &mockBatch{}, &mockBlobs{}, cfg, &mockManifest{}, "/tmp", &mockSignatureHandler{})

	writeMetadataTests := []struct {
		name              string
		cpImages          []v2alpha1.CopyImageSchema
		expectedImageRef  string
		expectedOrigin    string
		expectedItemCount int
	}{
		{
			name: "with targetRepo",
			cpImages: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://registry.redhat.io/ubi8/ubi:latest",
					Destination: "docker://localhost:5000/myregistry/custom-namespace/custom-image:latest",
					Origin:      "registry.redhat.io/ubi8/ubi:latest",
					Type:        v2alpha1.TypeGeneric,
				},
			},
			expectedImageRef:  "docker://localhost:5000/myregistry/custom-namespace/custom-image:latest",
			expectedOrigin:    "registry.redhat.io/ubi8/ubi:latest",
			expectedItemCount: 1,
		},
		{
			name: "with targetTag",
			cpImages: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://registry.redhat.io/ubi8/ubi:latest",
					Destination: "docker://localhost:5000/myregistry/ubi8/ubi:v1.0",
					Origin:      "registry.redhat.io/ubi8/ubi:latest",
					Type:        v2alpha1.TypeGeneric,
				},
			},
			expectedImageRef:  "docker://localhost:5000/myregistry/ubi8/ubi:v1.0",
			expectedOrigin:    "registry.redhat.io/ubi8/ubi:latest",
			expectedItemCount: 1,
		},
		{
			name: "with both targetRepo and targetTag",
			cpImages: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://registry.redhat.io/ubi8/ubi:latest",
					Destination: "docker://localhost:5000/myregistry/custom-namespace/custom-image:v2.0",
					Origin:      "registry.redhat.io/ubi8/ubi:latest",
					Type:        v2alpha1.TypeGeneric,
				},
			},
			expectedImageRef:  "docker://localhost:5000/myregistry/custom-namespace/custom-image:v2.0",
			expectedOrigin:    "registry.redhat.io/ubi8/ubi:latest",
			expectedItemCount: 1,
		},
		{
			name: "with targetTag for digest-only image",
			cpImages: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://sometest.registry.com/testns/test@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Destination: "docker://localhost:5000/myregistry/testns/test:v1.0",
					Origin:      "sometest.registry.com/testns/test@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
					Type:        v2alpha1.TypeGeneric,
				},
			},
			expectedImageRef:  "docker://localhost:5000/myregistry/testns/test:v1.0",
			expectedOrigin:    "sometest.registry.com/testns/test@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
			expectedItemCount: 1,
		},
		{
			name: "same origin with different targets should not be deduped",
			cpImages: []v2alpha1.CopyImageSchema{
				{
					Source:      "docker://registry.redhat.io/ubi8/ubi:latest",
					Destination: "docker://localhost:5000/myregistry/repo-a/ubi:latest",
					Origin:      "registry.redhat.io/ubi8/ubi:latest",
					Type:        v2alpha1.TypeGeneric,
				},
				{
					Source:      "docker://registry.redhat.io/ubi8/ubi:latest",
					Destination: "docker://localhost:5000/myregistry/repo-b/ubi:latest",
					Origin:      "registry.redhat.io/ubi8/ubi:latest",
					Type:        v2alpha1.TypeGeneric,
				},
			},
			expectedItemCount: 2,
		},
	}

	for _, tt := range writeMetadataTests {
		t.Run("WriteDeleteMetaData "+tt.name, func(t *testing.T) {
			err := di.WriteDeleteMetaData(context.Background(), tt.cpImages)
			assert.NoError(t, err)

			data, err := di.ReadDeleteMetaData()
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedItemCount, len(data.Items))
			if tt.expectedItemCount == 1 {
				assert.Equal(t, tt.expectedImageRef, data.Items[0].ImageReference)
				assert.Equal(t, tt.expectedOrigin, data.Items[0].ImageName)
			}
		})
	}

	deleteRegistryTests := []struct {
		name      string
		imageName string
		imageRef  string
	}{
		{
			name:      "with targetRepo",
			imageName: "registry.redhat.io/ubi8/ubi:latest",
			imageRef:  "docker://localhost:5000/myregistry/custom-namespace/custom-image:latest",
		},
		{
			name:      "with targetTag",
			imageName: "registry.redhat.io/ubi8/ubi:latest",
			imageRef:  "docker://localhost:5000/myregistry/ubi8/ubi:v1.0",
		},
		{
			name:      "with both targetRepo and targetTag",
			imageName: "registry.redhat.io/ubi8/ubi:latest",
			imageRef:  "docker://localhost:5000/myregistry/custom-namespace/custom-image:v2.0",
		},
		{
			name:      "with targetTag for digest-only image",
			imageName: "sometest.registry.com/testns/test@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea",
			imageRef:  "docker://localhost:5000/myregistry/testns/test:v1.0",
		},
	}

	for _, tt := range deleteRegistryTests {
		t.Run("DeleteRegistryImages "+tt.name, func(t *testing.T) {
			deleteImageList := v2alpha1.DeleteImageList{
				Items: []v2alpha1.DeleteItem{
					{
						ImageName:      tt.imageName,
						ImageReference: tt.imageRef,
						Type:           v2alpha1.TypeGeneric,
					},
				},
			}
			err := di.DeleteRegistryImages(deleteImageList)
			assert.NoError(t, err)
		})
	}
}

// OCPBUGS-105878: for a tag+digest image, --force-cache-delete must target the cache
// under the digest key, while the remote deletion keeps the human tag.
func TestDeleteRegistryImagesForceCacheDeleteTagAndDigest(t *testing.T) {
	log := clog.New("trace")

	global := &mirror.GlobalOptions{
		SecurePolicy:      false,
		WorkingDir:        consts.TestFolder,
		DeleteDestination: consts.DockerProtocol + "localhost:5000/myregistry",
		ForceCacheDelete:  true,
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
		Destination:         consts.DockerProtocol + "myregistry",
		Mode:                mirror.DiskToMirror,
		LocalStorageFQDN:    "localhost:8888",
	}

	const dgst = "sha256:44d75007b39e0e1bbf1bcfd0721245add54c54c3f83903f8926fb4bef6827aa2"

	var captured v2alpha1.CollectorSchema
	mb := &mockBatch{capturedSchema: &captured}
	di := New(log, opts, mb, &mockBlobs{}, v2alpha1.ImageSetConfiguration{}, &mockManifest{}, "/tmp", &mockSignatureHandler{})

	deleteImageList := v2alpha1.DeleteImageList{
		Items: []v2alpha1.DeleteItem{
			{
				ImageName:      "test.registry.io/testns/repo:v1@" + dgst,
				ImageReference: "docker://localhost:5000/myregistry/testns/repo:v1",
				Type:           v2alpha1.TypeGeneric,
			},
		},
	}

	err := di.DeleteRegistryImages(deleteImageList)
	assert.NoError(t, err)

	destinations := make([]string, 0, len(captured.AllImages))
	for _, img := range captured.AllImages {
		destinations = append(destinations, img.Destination)
	}

	// Remote deletion keeps the human tag (how it was mirrored to the destination).
	assert.Contains(t, destinations, "docker://localhost:5000/myregistry/testns/repo:v1")
	// Cache deletion must use the digest-encoded key (matching how M2D stored it).
	assert.Contains(t, destinations, "docker://localhost:8888/testns/repo:sha256-44d75007b39e0e1bbf1bcfd0721245add54c54c3f83903f8926fb4bef6827aa2")
	// And must NOT use the human tag for the cache (that entry does not exist -> orphaned manifest).
	assert.NotContains(t, destinations, "docker://localhost:8888/testns/repo:v1")
}

// mockBatch
type mockBatch struct {
	Fail bool
	// capturedSchema, when non-nil, receives the CollectorSchema passed to Worker so tests
	// can assert on the images (e.g. cache references) the delete flow builds.
	capturedSchema *v2alpha1.CollectorSchema
}

// mockBlobs
type mockBlobs struct {
	Fail bool
}

type mockManifest struct{}

func (o mockBatch) Worker(ctx context.Context, collectorSchema v2alpha1.CollectorSchema, opts mirror.CopyOptions) (v2alpha1.CollectorSchema, error) {
	if o.capturedSchema != nil {
		*o.capturedSchema = collectorSchema
	}
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

func (o *mockBlobs) GatherBlobs(ctx context.Context, image string, allowedPlatforms []string) (sets.Set[string], error) {
	res := sets.New("sha256:95ad8395795ee0460baf05458f669d3b865535f213f015519ef9a221a6a08280")
	if o.Fail {
		return nil, fmt.Errorf("forced error")
	}
	return res, nil
}

func (o mockManifest) GetOCIImageIndex(dir string) (*specv1.Index, error) {
	return nil, nil
}

func (o mockManifest) GetOCIImageManifest(file string) (*specv1.Manifest, error) {
	return nil, nil
}

func (o mockManifest) GetOperatorConfig(file string) (*v2alpha1.OperatorConfigSchema, error) {
	return &v2alpha1.OperatorConfigSchema{}, nil
}

func (o mockManifest) ExtractOCILayers(_ gcrv1.Image, toPath, label string) error {
	return nil
}

func (o mockManifest) GetReleaseSchema(filePath string) ([]v2alpha1.RelatedImage, error) {
	return []v2alpha1.RelatedImage{}, nil
}

func (o mockManifest) ConvertOCIIndexToSingleManifest(dir string, oci *specv1.Index) error {
	return nil
}

func (o mockManifest) ImageDigest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string) (string, error) {
	return "", nil
}

func (o mockManifest) ImageManifest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string, instanceDigest *digest.Digest) ([]byte, string, error) {
	return nil, "", nil
}

func (o mockManifest) GetManifestListDigests(ctx context.Context, sourceCtx *types.SystemContext, source string) ([]string, error) {
	return nil, nil
}

func (o mockManifest) GetOCIImageFromIndex(dir string) (gcrv1.Image, error) { //nolint:ireturn // as expected by go-containerregistry
	return nil, nil
}
