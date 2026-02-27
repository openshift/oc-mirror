package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/fake"
	"github.com/stretchr/testify/assert"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

func TestGetAllManifests(t *testing.T) {
	log := clog.New("debug")
	manifest := &Manifest{Log: log}

	// The failure cases are just json parsing errors
	t.Run("Testing GetImageManifest : should pass", func(t *testing.T) {
		expectedOCI := &v2alpha1.OCISchema{
			SchemaVersion: 2,
			MediaType:     "application/vnd.oci.image.manifest.v1+json",
			Config: v2alpha1.OCIManifest{
				MediaType: "application/vnd.oci.image.config.v1+json",
				Digest:    "sha256:18249fc55c217d5ecc543fb7cae29cdcbc8a7d94691e8a684f336abf75d01e64",
				Size:      1672,
			},
			Layers: []v2alpha1.OCIManifest{
				{
					MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					Digest:    "sha256:97da74cc6d8fa5d1634eb1760fd1da5c6048619c264c23e62d75f3bf6b8ef5c4",
					Size:      79524639,
				},
				{
					MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					Digest:    "sha256:d8190195889efb5333eeec18af9b6c82313edd4db62989bd3a357caca4f13f0e",
					Size:      1438,
				},
				{
					MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					Digest:    "sha256:f0f4937bc70fa7bf9afc1eb58400dbc646c9fd0c9f95cfdbfcdedd55f6fa0bcd",
					Size:      26654429,
				},
				{
					MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					Digest:    "sha256:833de2b0ccff7a77c31b4d2e3f96077b638aada72bfde75b5eddd5903dc11bb7",
					Size:      12374694,
				},
				{
					MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					Digest:    "sha256:911c7f3bfc1ca79614a05b77ad8b28e87f71026d41a34c8ea14b4f0a3657d0eb",
					Size:      25467095,
				},
				{
					MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					Digest:    "sha256:5b2ca04f694b70c8b41f1c2a40b7e95643181a1d037b115149ecc243324c513d",
					Size:      955593,
				},
			},
		}
		res, err := manifest.GetOCIImageManifest(filepath.Join(consts.TestFolder, "image-manifest.json"))
		assert.NoError(t, err)
		assert.Equal(t, expectedOCI, res)
	})

	// The failing cases are already covered by GetImageManifest tests
	t.Run("Testing GetImageIndex : should pass", func(t *testing.T) {
		expectedOCI := &v2alpha1.OCISchema{
			SchemaVersion: 2,
			Manifests: []v2alpha1.OCIManifest{
				{
					MediaType: "application/vnd.oci.image.manifest.v1+json",
					Digest:    "sha256:3ad31ff3302d352771aba8d1262e2d87cd4f796eac7195401c4d9c9af64e1627",
					Size:      1196,
				},
			},
		}
		res, err := manifest.GetOCIImageIndex(consts.TestFolder)
		assert.NoError(t, err)
		assert.Equal(t, expectedOCI, res)
	})

	t.Run("Testing GetOperatorConfig : should pass", func(t *testing.T) {
		expectedConfig := &v2alpha1.OperatorConfigSchema{
			Architecture: "amd64",
			Os:           "linux",
			Config: v2alpha1.OperatorConfig{
				Env: []string{
					"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
					"container=oci",
					"GODEBUG=x509ignoreCN=0,madvdontneed=1",
					"__doozer=merge",
					"BUILD_RELEASE=202301042354.p0.gf1dc3b6.assembly.stream",
					"BUILD_VERSION=v4.12.0",
					"OS_GIT_MAJOR=4",
					"OS_GIT_MINOR=12",
					"OS_GIT_PATCH=0",
					"OS_GIT_TREE_STATE=clean",
					"OS_GIT_VERSION=4.12.0-202301042354.p0.gf1dc3b6.assembly.stream-f1dc3b6",
					"SOURCE_GIT_TREE_STATE=clean",
					"OS_GIT_COMMIT=f1dc3b6",
					"SOURCE_DATE_EPOCH=1668720573",
					"SOURCE_GIT_COMMIT=f1dc3b6a6b7c5f5a85f94201ab90f9e03547a8a3",
					"SOURCE_GIT_TAG=v1.0.0-930-gf1dc3b6a",
					"SOURCE_GIT_URL=https://github.com/openshift/cluster-version-operator",
				},
				Entrypoint: []string{"/usr/bin/cluster-version-operator"},
			},
			RootFS: v2alpha1.OperatorRootFS{
				Type: "layers",
				DiffIds: []string{
					"sha256:e2e51ecd22dcbc318fb317f20dff685c6d54755d60a80b12ed290658864d45fd",
					"sha256:d3fbfed1573def1cd078186e307411a8929138baf65bdd0a02bcbdb451707f67",
					"sha256:10f5b8a5bcc334c27cb0c778c545ce6c3c77c79c9300e3bd5574dc7b12ef1e44",
					"sha256:50c36aad8a28245871d2cace944a4260d9de58b679a0e4ab285b481e123734eb",
					"sha256:8125f4f93a05cc066bb1660a63fcc1da73cf2fe10db2f512c79434e5c4108217",
					"sha256:eab47dc5c5fd4c2ad223788f24e27c437c95099c0e2b93cebf188347e1de9912",
				},
			},
		}
		res, err := manifest.GetOperatorConfig(filepath.Join(consts.TestFolder, "operator-config.json"))
		assert.NoError(t, err)
		// We don't care about some fields from the Schema (e.g. History), so
		// we cannot just compare the objects. Instead, we compare just the relevant fields
		// assert.Equal(t, expectedConfig, res)
		assert.Equal(t, expectedConfig.Config, res.Config, ".Config should match")
		assert.Equal(t, expectedConfig.RootFS, res.RootFS, ".RootFS should match")
		assert.Equal(t, expectedConfig.Architecture, res.Architecture, ".Architecture should match")
		assert.Equal(t, expectedConfig.Os, res.Os, ".Os should match")
	})

	t.Run("Testing GetReleaseSchema : should pass", func(t *testing.T) {
		expectedRI := []v2alpha1.RelatedImage{
			{
				Name:  "agent-installer-api-server",
				Image: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e4182331e",
				Type:  2,
			},
		}
		res, err := manifest.GetReleaseSchema(filepath.Join(consts.TestFolder, "release-schema.json"))
		assert.NoError(t, err)
		assert.Equal(t, expectedRI, res)
	})
}

func TestExtractOCILayers(t *testing.T) {
	log := clog.New("debug")
	manifest := &Manifest{Log: log}
	t.Run("Testing ExtractOCILayers : should pass", func(t *testing.T) {
		t.Run("when destination directory exists - no op", func(t *testing.T) {
			// this should do a nop (directory exists)
			destDir := filepath.Join(consts.TestFolder, "test-untar", "release-manifests")
			assert.DirExists(t, destDir, "directory should exist as precondition")
			err := manifest.ExtractOCILayers(&fake.FakeImage{}, filepath.Join(consts.TestFolder, "test-untar"), "release-manifests")
			assert.NoError(t, err, "should not fail: no op")
			assert.DirExists(t, destDir, "directory should still exist")
		})
		t.Run("when destination directory doesn't exist", func(t *testing.T) {
			destDir := t.TempDir()

			releaseManifestsLayerPath := filepath.Join(
				consts.TestFolder,
				"test-untar", "blobs", "sha256",
				"5b2ca04f694b70c8b41f1c2a40b7e95643181a1d037b115149ecc243324c513d",
			)
			img, err := crane.Append(empty.Image, releaseManifestsLayerPath)
			assert.NoError(t, err)

			err = manifest.ExtractOCILayers(img, destDir, "release-manifests")
			assert.NoError(t, err)

			manifestsDir := filepath.Join(destDir, "release-manifests")
			f, err := os.Open(manifestsDir)
			assert.NoError(t, err, "release-manifests dir should exist")
			defer f.Close()

			// We assume we are good if the file count matches
			const expectedManifestCount = 646
			flist, err := f.Readdirnames(-1)
			assert.NoError(t, err, "should read release-manifests contents")
			assert.Equal(t, expectedManifestCount, len(flist), "manifest count mismatch")
		})
	})
	t.Run("Testing ExtractOCILayers : should fail", func(t *testing.T) {
		t.Run("when cannot stat extract destination directory", func(t *testing.T) {
			// Create a directory we don't have permission to stat
			destDir := filepath.Join(t.TempDir(), "invalid-dest")
			err := os.Mkdir(destDir, 0o600)
			assert.NoError(t, err)

			err = manifest.ExtractOCILayers(&fake.FakeImage{}, destDir, "release-manifests")
			assert.Error(t, err)
			assert.Regexp(t, "extract directory: .*", err)
		})
	})
}
