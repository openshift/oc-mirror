package manifest

import (
	"os"
	"testing"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/common"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

func TestGetAllManifests(t *testing.T) {

	log := clog.New("debug")

	// these tests should cover over 80%
	t.Run("Testing GetImageIndex : should pass", func(t *testing.T) {
		manifest := &Manifest{Log: log}
		res, err := manifest.GetImageIndex(common.TestFolder)
		if err != nil {
			t.Fatalf("should not fail")
		}
		log.Debug("completed test  %v ", res)
	})

	t.Run("Testing GetImageManifest : should pass", func(t *testing.T) {
		manifest := &Manifest{Log: log}
		res, err := manifest.GetImageManifest(common.TestFolder + "image-manifest.json")
		if err != nil {
			t.Fatalf("should not fail")
		}
		log.Debug("completed test  %v ", res)
	})

	t.Run("Testing GetOperatorConfig : should pass", func(t *testing.T) {
		manifest := &Manifest{Log: log}
		res, err := manifest.GetOperatorConfig(common.TestFolder + "operator-config.json")
		if err != nil {
			t.Fatalf("should not fail")
		}
		log.Debug("completed test  %v ", res)
	})

	t.Run("Testing GetReleaseSchema : should pass", func(t *testing.T) {
		manifest := &Manifest{Log: log}
		res, err := manifest.GetReleaseSchema(common.TestFolder + "release-schema.json")
		if err != nil {
			log.Error(" %v ", err)
			t.Fatalf("should not fail")
		}
		log.Debug("completed test  %v ", res)
	})
}

func TestExtractOCILayers(t *testing.T) {

	log := clog.New("debug")
	t.Run("Testing ExtractOCILayers : should pass", func(t *testing.T) {
		oci := &v2alpha1.OCISchema{
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
			Layers: []v2alpha1.OCIManifest{
				{
					MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					Digest:    "sha256:d8190195889efb5333eeec18af9b6c82313edd4db62989bd3a357caca4f13f0e",
					Size:      1438,
				},
				{
					MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					Digest:    "sha256:5b2ca04f694b70c8b41f1c2a40b7e95643181a1d037b115149ecc243324c513d",
					Size:      955593,
				},
			},
		}
		manifest := &Manifest{Log: log}
		// this should do a nop (directory exists)
		err := manifest.ExtractLayersOCI(common.TestFolder+"test-untar/blobs/sha256", common.TestFolder+"test-untar", "release-manifests/", oci)
		if err != nil {
			t.Fatal("should not fail")
		}

		_, err = os.Stat(common.TestFolder + "hold-test-untar/release-manifests/")
		if err == nil {
			t.Fatalf("should fail")
		}

		err = manifest.ExtractLayersOCI(common.TestFolder+"test-untar/blobs/sha256", common.TestFolder+"hold-test-untar", "release-manifests/", oci)
		if err != nil {
			t.Fatalf("should not fail")
		}
		defer os.RemoveAll(common.TestFolder + "hold-test-untar")
		_, err = os.Stat(common.TestFolder + "hold-test-untar/release-manifests/")
		if err != nil {
			t.Fatalf("should not fail")
		}
	})
}
