package signature

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"go.podman.io/image/v5/manifest"
	"go.uber.org/mock/gomock"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	manifestmock "github.com/openshift/oc-mirror/v2/internal/pkg/manifest/mock"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

var multiArchManifest = `{
            "schemaVersion": 2,
            "mediaType": "application/vnd.oci.image.index.v1+json",
            "manifests": [
                {
                    "mediaType": "application/vnd.oci.image.manifest.v1+json",
                    "digest": "sha256:e033aa62f84267cf44de611acac2e76bfa4d2f0b6b2b61f1c4fecbefefde7159",
                    "size": 503,
                    "platform": {
                        "architecture": "amd64",
                        "os": "linux"
                    }
                },
                {
                    "mediaType": "application/vnd.oci.image.manifest.v1+json",
                    "digest": "sha256:02f29c270f30416a266571383098d7b98a49488723087fd917128045bcd1ca75",
                    "size": 503,
                    "platform": {
                        "architecture": "arm64",
                        "os": "linux"
                    }
                },
                {
                    "mediaType": "application/vnd.oci.image.manifest.v1+json",
                    "digest": "sha256:b15a2f174d803fd5fd7db0b3969c75cee0fe9131e0d8478f8c70ac01a4534869",
                    "size": 503,
                    "platform": {
                        "architecture": "s390x",
                        "os": "linux"
                    }
                },
                {
                    "mediaType": "application/vnd.oci.image.manifest.v1+json",
                    "digest": "sha256:832f20ad3d7e687c581b0a7d483174901d8bf22bb96c981b3f9da452817a754e",
                    "size": 503,
                    "platform": {
                        "architecture": "ppc64le",
                        "os": "linux"
                    }
                }
            ]
        }`

func TestSigstoreAttachmentTag(t *testing.T) {
	tests := []struct {
		name        string
		digest      digest.Digest
		expected    string
		expectError bool
	}{
		{
			name:        "Valid digest",
			digest:      "sha256:c8636a92b5665988f030ed0948225276fea7428f2fe1f227142c988dc409a515",
			expected:    "sha256-c8636a92b5665988f030ed0948225276fea7428f2fe1f227142c988dc409a515.sig",
			expectError: false,
		},
		{
			name:        "Invalid digest",
			digest:      "invalid-digest",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tag, err := SigstoreAttachmentTag(tt.digest)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, tag)
			}
		})
	}
}

func TestGetSignatureTag(t *testing.T) {
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

	log := clog.New("trace")
	opts := &mirror.CopyOptions{
		SrcImage: srcOpts,
	}

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	manifestMock := manifestmock.NewMockManifestInterface(mockCtrl)

	manifestMock.
		EXPECT().
		ImageManifest(gomock.Any(), gomock.Any(), gomock.Eq("docker://registry.example.com/test/single:latest"), gomock.Any()).
		Return([]byte("single-arch-manifest"), manifest.DockerV2Schema1MediaType, nil).
		AnyTimes()

	manifestMock.
		EXPECT().
		ImageManifest(gomock.Any(), gomock.Any(), gomock.Eq("docker://registry.example.com/test/multi:latest"), gomock.Any()).
		Return([]byte(multiArchManifest), manifest.DockerV2ListMediaType, nil).
		AnyTimes()

	manifestMock.
		EXPECT().
		ImageManifest(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, "", errors.New("unknown reference")).
		AnyTimes()

	handler := &SignatureHandler{
		opts:             opts,
		log:              log,
		ocmirrormanifest: manifestMock,
	}

	tests := []struct {
		name        string
		imgRef      string
		expected    []string
		expectError bool
	}{
		{
			name:        "Single arch image",
			imgRef:      "docker://registry.example.com/test/single:latest",
			expected:    []string{"sha256-3db1c382fbc0a0314a302f110b52bc12bf9d0d9b71fa7652ee849f0eff6781dc.sig"},
			expectError: false,
		},
		{
			name:        "Multi arch image",
			imgRef:      "docker://registry.example.com/test/multi:latest",
			expected:    []string{"sha256-c575d3422277328f5dde74a0ba463e1186108093329bdbc051f34856974575ea.sig", "sha256-e033aa62f84267cf44de611acac2e76bfa4d2f0b6b2b61f1c4fecbefefde7159.sig", "sha256-02f29c270f30416a266571383098d7b98a49488723087fd917128045bcd1ca75.sig", "sha256-b15a2f174d803fd5fd7db0b3969c75cee0fe9131e0d8478f8c70ac01a4534869.sig", "sha256-832f20ad3d7e687c581b0a7d483174901d8bf22bb96c981b3f9da452817a754e.sig"},
			expectError: false,
		},
		{
			name:        "Invalid reference",
			imgRef:      "invalid-reference",
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags, err := handler.GetSignatureTag(context.Background(), tt.imgRef)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, tags)
			}
		})
	}
}

func TestMultiArchSigTags(t *testing.T) {
	log := clog.New("trace")
	handler := &SignatureHandler{
		log: log,
	}

	tests := []struct {
		name        string
		manifest    []byte
		mime        string
		digest      digest.Digest
		expected    []string
		expectError bool
	}{
		{
			name:        "Valid manifest list",
			manifest:    []byte(multiArchManifest),
			mime:        manifest.DockerV2ListMediaType,
			digest:      "sha256:c575d3422277328f5dde74a0ba463e1186108093329bdbc051f34856974575ea",
			expected:    []string{"sha256-c575d3422277328f5dde74a0ba463e1186108093329bdbc051f34856974575ea.sig", "sha256-e033aa62f84267cf44de611acac2e76bfa4d2f0b6b2b61f1c4fecbefefde7159.sig", "sha256-02f29c270f30416a266571383098d7b98a49488723087fd917128045bcd1ca75.sig", "sha256-b15a2f174d803fd5fd7db0b3969c75cee0fe9131e0d8478f8c70ac01a4534869.sig", "sha256-832f20ad3d7e687c581b0a7d483174901d8bf22bb96c981b3f9da452817a754e.sig"},
			expectError: false,
		},
		{
			name:        "Invalid manifest list",
			manifest:    []byte(`{"manifests":[{"digest":"sha256:abc123"},{"digest":"sha256:def456"}]}`),
			mime:        manifest.DockerV2ListMediaType,
			digest:      "sha256:list123",
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags, err := handler.multiArchSigTags(tt.manifest, tt.mime, tt.digest)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, tags)
			}
		})
	}
}

func TestSingleArchSigTags(t *testing.T) {
	log := clog.New("trace")
	handler := &SignatureHandler{
		log: log,
	}

	tests := []struct {
		name        string
		digest      digest.Digest
		expected    []string
		expectError bool
	}{
		{
			name:        "Valid digest",
			digest:      "sha256:c8636a92b5665988f030ed0948225276fea7428f2fe1f227142c988dc409a515",
			expected:    []string{"sha256-c8636a92b5665988f030ed0948225276fea7428f2fe1f227142c988dc409a515.sig"},
			expectError: false,
		},
		{
			name:        "Invalid digest",
			digest:      "sha256:abc123",
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags, err := handler.singleArchSigTags(tt.digest)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, tags)
			}
		})
	}
}
