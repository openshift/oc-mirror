package batch

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/registry/api/errcode"
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type BatchSchema struct {
	Images v2alpha1.CollectorSchema
}

func TestChannelConcurrentWorker(t *testing.T) {
	log := clog.New("trace")

	global := &mirror.GlobalOptions{SecurePolicy: false, Quiet: false}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	m2dopts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Destination:         "oci:test",
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
		Function:            "copy",
	}
	d2mopts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Destination:         "oci:test",
		Dev:                 false,
		Mode:                mirror.DiskToMirror,
		Function:            "copy",
	}
	m2mopts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Destination:         "oci:test",
		Dev:                 false,
		Mode:                mirror.MirrorToMirror,
		Function:            "copy",
	}
	deleteopts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Destination:         "oci:test",
		Dev:                 false,
		Mode:                mirror.DiskToMirror,
		Function:            "copy",
	}
	tempDir := t.TempDir()
	relatedImages := []v2alpha1.CopyImageSchema{
		{Source: "docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testa", Type: v2alpha1.TypeOCPRelease},
		{Source: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testb", Type: v2alpha1.TypeOCPReleaseContent},
		{Source: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testc", Type: v2alpha1.TypeOperatorBundle},
		{Source: "docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testd", Type: v2alpha1.TypeOperatorCatalog},
		{Source: "docker://registry/name/namespace/sometestimage-e@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-e@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:teste", Type: v2alpha1.TypeCincinnatiGraph},
		{Source: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testf", Type: v2alpha1.TypeOperatorRelatedImage},
		{Source: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testg", Type: v2alpha1.TypeCincinnatiGraph},
		{Source: "docker://registry/name/namespace/sometestimage-h@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-h@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testh", Type: v2alpha1.TypeGeneric},
		{Source: "docker://registry/name/namespace/sometestimage-i@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-i@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testi", Type: v2alpha1.TypeGeneric},
	}

	timestampStr := time.Now().Format("20060102_150405")

	collectedImages := v2alpha1.CollectorSchema{AllImages: relatedImages, TotalReleaseImages: 4, TotalOperatorImages: 3, TotalAdditionalImages: 2}
	t.Run("Testing m2m Worker - no errors: should pass", func(t *testing.T) {
		mirrorMock := new(MirrorMock)
		mirrorMock.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		w := New(ChannelConcurrentWorker, log, tempDir, mirrorMock, uint(8), timestampStr)

		copiedImages, err := w.Worker(context.Background(), collectedImages, m2mopts)
		if err != nil {
			t.Fatal("should pass")
		}
		assert.ElementsMatch(t, relatedImages, copiedImages.AllImages)
	})

	t.Run("Testing m2d Worker - no errors: should pass", func(t *testing.T) {
		mirrorMock := new(MirrorMock)
		mirrorMock.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

		w := New(ChannelConcurrentWorker, log, tempDir, mirrorMock, uint(8), timestampStr)

		copiedImages, err := w.Worker(context.Background(), collectedImages, m2dopts)
		if err != nil {
			t.Fatal("should pass")
		}
		assert.ElementsMatch(t, relatedImages, copiedImages.AllImages)
	})
	t.Run("Testing d2m Worker - no errors: should pass", func(t *testing.T) {
		mirrorMock := new(MirrorMock)
		mirrorMock.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		w := New(ChannelConcurrentWorker, log, tempDir, mirrorMock, uint(8), timestampStr)

		copiedImages, err := w.Worker(context.Background(), collectedImages, d2mopts)
		if err != nil {
			t.Fatal("should pass")
		}
		assert.ElementsMatch(t, relatedImages, copiedImages.AllImages)
	})

	t.Run("Testing delete Worker - no errors: should pass", func(t *testing.T) {
		mirrorMock := new(MirrorMock)
		mirrorMock.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		w := New(ChannelConcurrentWorker, log, tempDir, mirrorMock, uint(8), timestampStr)

		copiedImages, err := w.Worker(context.Background(), collectedImages, deleteopts)
		if err != nil {
			t.Fatal("should pass")
		}
		assert.ElementsMatch(t, relatedImages, copiedImages.AllImages)
	})
	t.Run("Testing m2d Worker - single error on operator: should return safe error", func(t *testing.T) {
		mirrorMock := new(MirrorMock)
		mirrorMock.On("Run", mock.Anything, "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", mock.Anything, mock.Anything, mock.Anything).Return(errcode.Error{Code: errcode.ErrorCodeUnauthorized, Message: "unauthorized"})
		mirrorMock.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		w := New(ChannelConcurrentWorker, log, tempDir, mirrorMock, uint(8), timestampStr)

		copiedImages, err := w.Worker(context.Background(), collectedImages, m2dopts)
		if err == nil {
			t.Fatal("should return safe error")
		}

		assert.Equal(t, len(relatedImages)-1, len(copiedImages.AllImages))
	})
	t.Run("Testing d2m Worker - 1 err release / 2 errors: should return unsafe error", func(t *testing.T) {
		mirrorMock := new(MirrorMock)
		mirrorMock.On("Run", mock.Anything, "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", mock.Anything, mock.Anything, mock.Anything).Return(errcode.Error{Code: errcode.ErrorCodeUnauthorized, Message: "unauthorized"})
		mirrorMock.On("Run", mock.Anything, "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", mock.Anything, mock.Anything, mock.Anything).Return(errcode.Error{Code: errcode.ErrorCodeManifestUnknown, Message: "Manifest Unknown"})
		mirrorMock.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		w := New(ChannelConcurrentWorker, log, tempDir, mirrorMock, uint(8), timestampStr)

		copiedImages, err := w.Worker(context.Background(), collectedImages, d2mopts)
		if err == nil {
			t.Fatal("should return unsafe error")
		}

		assert.GreaterOrEqual(t, len(relatedImages), len(copiedImages.AllImages))
	})
	t.Run("Testing d2m Worker - 2 errors: should return safe error", func(t *testing.T) {
		mirrorMock := new(MirrorMock)
		mirrorMock.On("Run", mock.Anything, "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", mock.Anything, mock.Anything, mock.Anything).Return(errcode.Error{Code: errcode.ErrorCodeUnauthorized, Message: "unauthorized"})
		mirrorMock.On("Run", mock.Anything, "docker://registry/name/namespace/sometestimage-h@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", mock.Anything, mock.Anything, mock.Anything).Return(errcode.Error{Code: errcode.ErrorCodeManifestUnknown, Message: "Manifest Unknown"})
		mirrorMock.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		w := New(ChannelConcurrentWorker, log, tempDir, mirrorMock, uint(8), timestampStr)

		copiedImages, err := w.Worker(context.Background(), collectedImages, d2mopts)
		if err == nil {
			t.Fatal("should return safe error")
		}

		assert.GreaterOrEqual(t, len(relatedImages), len(copiedImages.AllImages))
	})

	t.Run("Testing m2d Worker - single error on operator related image: bundle of the related image should skip but fail in the end", func(t *testing.T) {
		relatedImages := []v2alpha1.CopyImageSchema{
			{Source: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testf", Type: v2alpha1.TypeOperatorRelatedImage},
			{Source: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testc", Type: v2alpha1.TypeOperatorBundle},
		}

		copyImageSchemaMap := &v2alpha1.CopyImageSchemaMap{OperatorsByImage: make(map[string]map[string]struct{}), BundlesByImage: make(map[string]map[string]string)}

		copyImageSchemaMap.OperatorsByImage[relatedImages[0].Origin] = make(map[string]struct{})
		copyImageSchemaMap.OperatorsByImage[relatedImages[0].Origin]["operator-c"] = struct{}{}
		copyImageSchemaMap.BundlesByImage[relatedImages[0].Origin] = make(map[string]string)
		copyImageSchemaMap.BundlesByImage[relatedImages[0].Origin][strings.Split(relatedImages[1].Origin, "://")[1]] = "bundle-c"

		collectedImages := v2alpha1.CollectorSchema{AllImages: relatedImages, TotalOperatorImages: 2, CopyImageSchemaMap: *copyImageSchemaMap}

		mirrorMock := new(MirrorMock)
		mirrorMock.On("Run", mock.Anything, "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", mock.Anything, mock.Anything, mock.Anything).Return(errcode.Error{Code: errcode.ErrorCodeUnauthorized, Message: "unauthorized"})
		mirrorMock.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		w := New(ChannelConcurrentWorker, log, tempDir, mirrorMock, uint(1), timestampStr)

		_, err := w.Worker(context.Background(), collectedImages, m2dopts)
		assert.Error(t, err)

		errorMsg := err.Error()

		pattern := `/tmp/[^\s]+`
		regex, err := regexp.Compile(pattern)
		assert.NoError(t, err)

		filePath := regex.FindString(errorMsg)
		assert.NotEmpty(t, filePath)

		fileContent, err := os.ReadFile(filePath)
		assert.NoError(t, err)

		expectedMsg := "error mirroring image %s (Operator bundles: [bundle-c] - Operators: [operator-c]) error: unauthorized: unauthorized"
		assert.Contains(t, string(fileContent), fmt.Sprintf(expectedMsg, relatedImages[0].Origin))
	})
}

func TestSplitImagesToBatches(t *testing.T) {
	type testCase struct {
		name            string
		images          v2alpha1.CollectorSchema
		expectedBatches []BatchSchema
	}
	testCases := []testCase{
		{
			name: "0 batches when no images",
			images: v2alpha1.CollectorSchema{
				AllImages: []v2alpha1.CopyImageSchema{},
			},
			expectedBatches: []BatchSchema{},
		},
		{
			name: "1 batch when 3 images",
			images: v2alpha1.CollectorSchema{
				AllImages: []v2alpha1.CopyImageSchema{
					{Source: "docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
				},
			},
			expectedBatches: []BatchSchema{
				{
					Images: v2alpha1.CollectorSchema{
						AllImages: []v2alpha1.CopyImageSchema{
							{Source: "docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
						},
					},
				},
			},
		},
		{
			name: "1 batch when 8 images",
			images: v2alpha1.CollectorSchema{
				AllImages: []v2alpha1.CopyImageSchema{
					{Source: "docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-e@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-h@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					// {Source: "docker://registry/name/namespace/sometestimage-i@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
				},
			},
			expectedBatches: []BatchSchema{
				{
					Images: v2alpha1.CollectorSchema{
						AllImages: []v2alpha1.CopyImageSchema{
							{Source: "docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-e@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-h@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
						},
					},
				},
			},
		},
		{
			name: "2 batch when 16 images",
			images: v2alpha1.CollectorSchema{
				AllImages: []v2alpha1.CopyImageSchema{
					{Source: "docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-e@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-h@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-i@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-j@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-k@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-l@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-m@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-n@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-o@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-p@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
				},
			},
			expectedBatches: []BatchSchema{
				{
					Images: v2alpha1.CollectorSchema{
						AllImages: []v2alpha1.CopyImageSchema{
							{Source: "docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-e@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-h@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
						},
					},
				},
				{
					Images: v2alpha1.CollectorSchema{
						AllImages: []v2alpha1.CopyImageSchema{
							{Source: "docker://registry/name/namespace/sometestimage-i@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-j@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-k@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-l@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-m@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-n@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-o@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-p@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
						},
					},
				},
			},
		},
		{
			name: "2 batch when 9 images",
			images: v2alpha1.CollectorSchema{
				AllImages: []v2alpha1.CopyImageSchema{
					{Source: "docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-e@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-h@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
					{Source: "docker://registry/name/namespace/sometestimage-i@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
				},
			},
			expectedBatches: []BatchSchema{
				{
					Images: v2alpha1.CollectorSchema{
						AllImages: []v2alpha1.CopyImageSchema{
							{Source: "docker://registry/name/namespace/sometestimage-a@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-b@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-c@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-d@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-e@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-f@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
							{Source: "docker://registry/name/namespace/sometestimage-h@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
						},
					},
				},
				{
					Images: v2alpha1.CollectorSchema{
						AllImages: []v2alpha1.CopyImageSchema{
							{Source: "docker://registry/name/namespace/sometestimage-i@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:test"},
						},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			batches := splitImagesToBatches(tc.images, 8)
			assert.ElementsMatch(t, tc.expectedBatches, batches)
		})
	}
}

func TestShouldSkipImage(t *testing.T) {
	type testCase struct {
		caseName          string
		img               v2alpha1.CopyImageSchema
		mode              string
		errArray          []mirrorErrorSchema
		updateURLOverride string
		expectToSkip      bool
		expectedError     bool
	}
	testCases := []testCase{
		{
			caseName:          "ShouldSkipImage GraphImage - M2M - no UpdateURLOVerride : should skip",
			img:               v2alpha1.CopyImageSchema{Source: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testg", Type: v2alpha1.TypeCincinnatiGraph},
			mode:              mirror.MirrorToMirror,
			errArray:          []mirrorErrorSchema{},
			updateURLOverride: "",
			expectedError:     false,
			expectToSkip:      true,
		},
		{
			caseName:          "ShouldSkipImage GraphImage - M2D - no UpdateURLOVerride : should skip",
			img:               v2alpha1.CopyImageSchema{Source: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testg", Type: v2alpha1.TypeCincinnatiGraph},
			mode:              mirror.MirrorToDisk,
			errArray:          []mirrorErrorSchema{},
			updateURLOverride: "",
			expectedError:     false,
			expectToSkip:      true,
		},
		{
			caseName:          "ShouldSkipImage GraphImage - D2M - no UpdateURLOVerride : should not skip",
			img:               v2alpha1.CopyImageSchema{Source: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testg", Type: v2alpha1.TypeCincinnatiGraph},
			mode:              mirror.DiskToMirror,
			errArray:          []mirrorErrorSchema{},
			updateURLOverride: "",
			expectedError:     false,
			expectToSkip:      false,
		},
		{
			caseName:          "ShouldSkipImage GraphImage - M2M - UpdateURLOVerride : should not skip",
			img:               v2alpha1.CopyImageSchema{Source: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testg", Type: v2alpha1.TypeCincinnatiGraph},
			mode:              mirror.MirrorToMirror,
			errArray:          []mirrorErrorSchema{},
			updateURLOverride: "https://localhost.localdomain:3443/graph",
			expectedError:     false,
			expectToSkip:      false,
		},
		{
			caseName:          "ShouldSkipImage GraphImage - M2D - UpdateURLOVerride : should not skip",
			img:               v2alpha1.CopyImageSchema{Source: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testg", Type: v2alpha1.TypeCincinnatiGraph},
			mode:              mirror.MirrorToDisk,
			errArray:          []mirrorErrorSchema{},
			updateURLOverride: "https://localhost.localdomain:3443/graph",
			expectedError:     false,
			expectToSkip:      false,
		},
		{
			caseName:          "ShouldSkipImage GraphImage - D2M - UpdateURLOVerride : should not skip",
			img:               v2alpha1.CopyImageSchema{Source: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Origin: "docker://registry/name/namespace/sometestimage-g@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", Destination: "oci:testg", Type: v2alpha1.TypeCincinnatiGraph},
			mode:              mirror.DiskToMirror,
			errArray:          []mirrorErrorSchema{},
			updateURLOverride: "https://localhost.localdomain:3443/graph",
			expectedError:     false,
			expectToSkip:      false,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			if testCase.updateURLOverride != "" {
				t.Setenv("UPDATE_URL_OVERRIDE", testCase.updateURLOverride)
			}

			skip, err := shouldSkipImageOld(testCase.img, testCase.mode, testCase.errArray)
			if testCase.expectedError && err == nil {
				t.Error("expected to fail with error, but no error was returned")
			}
			if !testCase.expectedError && err != nil {
				t.Errorf("unexpected failure : %v", err)
			}
			assert.Equal(t, testCase.expectToSkip, skip)
		})
	}
}

type MirrorMock struct {
	mock.Mock
}

func (o *MirrorMock) Run(ctx context.Context, src, dest string, mode mirror.Mode, opts *mirror.CopyOptions) error {
	args := o.Called(ctx, src, dest, mode, opts)
	return args.Error(0)
}

func (o *MirrorMock) Check(ctx context.Context, image string, opts *mirror.CopyOptions, asCopySrc bool) (bool, error) {
	return true, nil
}

// later, we can consider making this func smarter:
// by putting related images, release content images first
// and deferring operator bundle images, second
// and lastly release images and catalog images
// CLID-133 + CLID-98
func splitImagesToBatches(images v2alpha1.CollectorSchema, maxBatchSize int) []BatchSchema {
	imgsCount := len(images.AllImages)
	if imgsCount == 0 {
		return []BatchSchema{}
	}
	if imgsCount <= maxBatchSize {
		return []BatchSchema{
			{
				Images: images,
			},
		}
	} else {
		batches := []BatchSchema{}
		for index := 0; index < imgsCount; index += maxBatchSize {
			batch := BatchSchema{}
			batchSize := min(maxBatchSize, imgsCount-index)
			batch.Images = v2alpha1.CollectorSchema{
				AllImages: images.AllImages[index : index+batchSize],
			}
			batches = append(batches, batch)
		}
		return batches
	}
}

// shouldSkipImage helps determine whether the batch should perform the mirroring of the image
// or if the image should be skipped.
func shouldSkipImageOld(img v2alpha1.CopyImageSchema, mode string, errArray []mirrorErrorSchema) (bool, error) {
	// In MirrorToMirror and MirrorToDisk, the release collector will generally build and push the graph image
	// to the destination registry (disconnected registry or cache resp.)
	// Therefore this image can be skipped.
	// OCPBUGS-38037: The only exception to this is in the enclave environment. Enclave environment is detected by the presence
	// of env var UPDATE_URL_OVERRIDE.
	// When in enclave environment, release collector cannot build nor push the graph image. Therefore graph image
	// should not be skipped.
	updateURLOverride := os.Getenv("UPDATE_URL_OVERRIDE")
	if img.Type == v2alpha1.TypeCincinnatiGraph && (mode == mirror.MirrorToDisk || mode == mirror.MirrorToMirror) && len(updateURLOverride) == 0 {
		return true, nil
	}

	if img.Type == v2alpha1.TypeOperatorBundle {
		for _, err := range errArray {
			bundleImage := img.Origin
			if strings.Contains(bundleImage, "://") {
				bundleImage = strings.Split(img.Origin, "://")[1]
			}

			if err.bundles != nil && err.bundles.Has(bundleImage) {
				return true, fmt.Errorf(skippingMsg, img.Origin)
			}
		}
	}

	return false, nil
}
