package clusterresources

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	updateservicev1 "github.com/openshift/oc-mirror/v2/pkg/clusterresources/updateservice/v1"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/yaml"
)

func TestIDMSGenerator(t *testing.T) {
	log := clog.New("trace")

	tmpDir := t.TempDir()
	workingDir := tmpDir + "/working-dir"

	defer os.RemoveAll(tmpDir)

	imageList := []v1alpha3.CopyImageSchema{
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:6d76ffca7a233213325907bae611e835b49c5b933095be1328351f4f5fc67615",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:6d76ffca7a233213325907bae611e835b49c5b933095be1328351f4f5fc67615",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:6d76ffca7a233213325907bae611e835b49c5b933095be1328351f4f5fc67615",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4c181f5cbea53472acd9695232f77a0933a73f7f40f543cbd48dff00e6f03090",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4c181f5cbea53472acd9695232f77a0933a73f7f40f543cbd48dff00e6f03090",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4c181f5cbea53472acd9695232f77a0933a73f7f40f543cbd48dff00e6f03090",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:ff8ef167b679606b17baf75d94a02589048849b550c4cc17d36506a28f22b29c",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:ff8ef167b679606b17baf75d94a02589048849b550c4cc17d36506a28f22b29c",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:ff8ef167b679606b17baf75d94a02589048849b550c4cc17d36506a28f22b29c",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:d62f2612d3b9618a04ac0dea3ee2e1dec63d8fbe2279e86aa2a605d8755f2b8f",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:d62f2612d3b9618a04ac0dea3ee2e1dec63d8fbe2279e86aa2a605d8755f2b8f",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:d62f2612d3b9618a04ac0dea3ee2e1dec63d8fbe2279e86aa2a605d8755f2b8f",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
		},
	}

	t.Run("Testing IDMSGenerator - Disk to Mirror : should pass", func(t *testing.T) {
		cr := &ClusterResourcesGenerator{
			Log:        log,
			WorkingDir: workingDir,
		}
		err := cr.IDMSGenerator(imageList)
		if err != nil {
			t.Fatalf("should not fail")
		}

		_, err = os.Stat(filepath.Join(workingDir, clusterResourcesDir))
		if err != nil {
			t.Fatalf("output folder should exist")
		}

		idmsFiles, err := os.ReadDir(filepath.Join(workingDir, clusterResourcesDir))
		if err != nil {
			t.Fatalf("ls output folder should not fail")
		}

		if len(idmsFiles) != 1 {
			t.Fatalf("output folder should contain 1 idms yaml file")
		}
		// check idmsFile has a name that is
		//compliant with Kubernetes requested
		// RFC-1035 + RFC1123
		// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-label-names
		customResourceName := strings.TrimSuffix(idmsFiles[0].Name(), ".yaml")
		if !isValidRFC1123(customResourceName) {
			t.Fatalf("IDMS custom resource name %s doesn't  respect RFC1123", idmsFiles[0].Name())
		}
	})
}

func isValidRFC1123(name string) bool {
	// Regular expression to match RFC1123 compliant names
	rfc1123Regex := "^[a-zA-Z0-9][-a-zA-Z0-9]*[a-zA-Z0-9]$"
	match, _ := regexp.MatchString(rfc1123Regex, name)
	return match && len(name) <= 63
}

func TestGenerateImageMirrors(t *testing.T) {

	imageList := []v1alpha3.CopyImageSchema{
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:6d76ffca7a233213325907bae611e835b49c5b933095be1328351f4f5fc67615",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:6d76ffca7a233213325907bae611e835b49c5b933095be1328351f4f5fc67615",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:6d76ffca7a233213325907bae611e835b49c5b933095be1328351f4f5fc67615",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4c181f5cbea53472acd9695232f77a0933a73f7f40f543cbd48dff00e6f03090",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4c181f5cbea53472acd9695232f77a0933a73f7f40f543cbd48dff00e6f03090",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4c181f5cbea53472acd9695232f77a0933a73f7f40f543cbd48dff00e6f03090",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:ff8ef167b679606b17baf75d94a02589048849b550c4cc17d36506a28f22b29c",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:ff8ef167b679606b17baf75d94a02589048849b550c4cc17d36506a28f22b29c",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:ff8ef167b679606b17baf75d94a02589048849b550c4cc17d36506a28f22b29c",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:d62f2612d3b9618a04ac0dea3ee2e1dec63d8fbe2279e86aa2a605d8755f2b8f",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:d62f2612d3b9618a04ac0dea3ee2e1dec63d8fbe2279e86aa2a605d8755f2b8f",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:d62f2612d3b9618a04ac0dea3ee2e1dec63d8fbe2279e86aa2a605d8755f2b8f",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
		},
	}

	t.Run("Testing GenerateImageMirrors - Disk to Mirror : should have 1 namespace", func(t *testing.T) {

		mirrors, err := generateImageMirrors(imageList)
		if err != nil {
			t.Fatalf("should not fail")
		}
		if len(mirrors) != 1 {
			t.Fatal("should contain 1 source")
		}

		idm := mirrors["quay.io/openshift-release-dev"]
		if len(idm) != 1 {
			t.Fatalf("should contain 1 mirror for source quay.io/openshift-release-dev. Found %d", len(idm))
		}

		if idm[0] != "myregistry/mynamespace/quay.io/openshift-release-dev" {
			t.Fatalf("returned mirror does not match expected: %s", idm[0])
		}
	})
}

func TestUpdateServiceGenerator(t *testing.T) {
	log := clog.New("trace")

	tmpDir := t.TempDir()
	workingDir := tmpDir + "/working-dir"

	releaseImage := "quay.io/openshift-release-dev/ocp-release:4.13.10-x86_64"
	graphImage := "localhost:5000/openshift/graph-image:latest"

	t.Run("Testing IDMSGenerator - Disk to Mirror : should pass", func(t *testing.T) {
		cr := &ClusterResourcesGenerator{
			Log:        log,
			WorkingDir: workingDir,
		}
		err := cr.UpdateServiceGenerator(graphImage, releaseImage)
		if err != nil {
			t.Fatalf("should not fail")
		}

		_, err = os.Stat(filepath.Join(workingDir, clusterResourcesDir))
		if err != nil {
			t.Fatalf("output folder should exist")
		}

		resourceFiles, err := os.ReadDir(filepath.Join(workingDir, clusterResourcesDir))
		if err != nil {
			t.Fatalf("ls output folder should not fail")
		}

		if len(resourceFiles) != 1 {
			t.Fatalf("output folder should contain 1 updateservice.yaml file")
		}

		assert.Equal(t, updateServiceFilename, resourceFiles[0].Name())

		// Read the contents of resourceFiles[0]
		filePath := filepath.Join(workingDir, clusterResourcesDir, resourceFiles[0].Name())
		fileContents, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		actualOSUS := updateservicev1.UpdateService{}
		err = yaml.Unmarshal(fileContents, &actualOSUS)
		if err != nil {
			t.Fatalf("failed to unmarshall file: %v", err)
		}

		assert.Equal(t, graphImage, actualOSUS.Spec.GraphDataImage)
		assert.Equal(t, "quay.io/openshift-release-dev/ocp-release", actualOSUS.Spec.Releases)
	})
}
