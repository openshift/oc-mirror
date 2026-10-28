package operator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBundleManifestsSubdir(t *testing.T) {
	// The label is read from the bundle image, so a hostile catalog controls
	// it. Anything that escapes the extraction directory must be rejected.
	accepted := map[string]string{
		"manifests":   "manifests",
		"manifests/":  "manifests",
		"./manifests": "manifests",
		"/manifests":  "manifests",
		"nested/mani": "nested/mani",
	}
	for label, want := range accepted {
		t.Run("accepts "+label, func(t *testing.T) {
			got, err := bundleManifestsSubdir(label)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}

	// "a/../manifests" is contained only lexically: if "a" is a symlink from
	// the bundle, ".." escapes. It must be rejected, not cleaned away.
	for _, label := range []string{"..", "../", "../../etc", "a/../../b", ".", "", "../manifests", "a/../manifests", "manifests/../.."} {
		t.Run("rejects "+label, func(t *testing.T) {
			_, err := bundleManifestsSubdir(label)
			require.Error(t, err, "a label escaping the extraction directory must not be used")
			assert.Contains(t, err.Error(), "invalid manifests directory")
		})
	}
}

func TestFindClusterServiceVersion(t *testing.T) {
	// ReadDir returns entries sorted, so the decoy is visited first.
	const decoy = "aaa-csv-defaults.yaml"
	const real = "zzz.clusterserviceversion.yaml"

	realCSV := "apiVersion: operators.coreos.com/v1alpha1\n" +
		"kind: ClusterServiceVersion\n" +
		"metadata:\n  name: real-operator.v1.0.0\n"
	kindless := "someKey: someValue\notherKey: 2\n"

	t.Run("a kindless file does not shadow the real CSV", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, decoy), []byte(kindless), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, real), []byte(realCSV), 0o600))

		csv, err := findClusterServiceVersion(dir)
		require.NoError(t, err)
		assert.Equal(t, "real-operator.v1.0.0", csv.Name,
			"an empty CSV would make every related image look like an operand")
	})

	t.Run("a kindless file alone is not a CSV", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, decoy), []byte(kindless), 0o600))

		_, err := findClusterServiceVersion(dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no ClusterServiceVersion found")
	})

	csv := `apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: test-operator.v1.0.0
spec:
  install:
    spec:
      deployments:
        - name: controller
          spec:
            template:
              spec:
                containers:
                  - name: manager
                    image: registry.example.com/controller@sha256:aaa
                initContainers:
                  - name: init
                    image: registry.example.com/init@sha256:bbb
  relatedImages:
    - name: operand
      image: registry.example.com/operand@sha256:ccc
`

	t.Run("parses the CSV and finds every container image", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "test.clusterserviceversion.yaml"), []byte(csv), 0o600))

		parsed, err := findClusterServiceVersion(dir)
		require.NoError(t, err)

		images := map[string]struct{}{}
		for _, dep := range parsed.Spec.InstallStrategy.StrategySpec.DeploymentSpecs {
			for _, c := range dep.Spec.Template.Spec.Containers {
				images[c.Image] = struct{}{}
			}
			for _, c := range dep.Spec.Template.Spec.InitContainers {
				images[c.Image] = struct{}{}
			}
		}
		assert.Contains(t, images, "registry.example.com/controller@sha256:aaa")
		assert.Contains(t, images, "registry.example.com/init@sha256:bbb")
		assert.NotContains(t, images, "registry.example.com/operand@sha256:ccc",
			"a related image that no deployment runs is an operand")
	})

	t.Run("reports when there is no CSV", func(t *testing.T) {
		_, err := findClusterServiceVersion(t.TempDir())
		assert.Error(t, err)
	})
}
