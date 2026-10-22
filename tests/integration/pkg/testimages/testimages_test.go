package testimages

import (
	"strings"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

const fooBundleDir = "../../testdata/images/operator/bundles/foo/foo-bundle-v0.3.1"

func TestBundleImageIsReproducible(t *testing.T) {
	inventory := newInventory()
	inventory.add(operatorRepo+":"+operandTag, v1.Hash{Algorithm: "sha256", Hex: strings.Repeat("a", 64)})

	build := func() string {
		labels, err := bundleLabels(fooBundleDir)
		if err != nil {
			t.Fatal(err)
		}
		files, err := filesFromDir(fooBundleDir, "", []string{"manifests", "metadata"}, inventory.render)
		if err != nil {
			t.Fatal(err)
		}
		img, err := buildImage(imageSpec{platform: linuxAMD64, labels: labels, layers: [][]layerFile{files}})
		if err != nil {
			t.Fatal(err)
		}
		digest, err := img.Digest()
		if err != nil {
			t.Fatal(err)
		}
		return digest.String()
	}

	if first, second := build(), build(); first != second {
		t.Errorf("building the same bundle twice gave different digests: %s, %s", first, second)
	}
}

func TestBundleLabels(t *testing.T) {
	labels, err := bundleLabels(fooBundleDir)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"operators.operatorframework.io.bundle.mediatype.v1": "registry+v1",
		"operators.operatorframework.io.bundle.package.v1":   "foo",
		"operators.operatorframework.io.bundle.channels.v1":  "beta",
	} {
		if labels[key] != want {
			t.Errorf("label %s = %q, want %q", key, labels[key], want)
		}
	}
}

func TestRenderDigest(t *testing.T) {
	inventory := newInventory()
	digest := v1.Hash{Algorithm: "sha256", Hex: strings.Repeat("b", 64)}
	inventory.add(operatorRepo+":"+operandTag, digest)

	out, err := inventory.render("test", []byte(`image: localhost:5001/oc-mirror/oc-mirror-dev@{{ digest "oc-mirror/oc-mirror-dev:operand" }}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := "image: localhost:5001/oc-mirror/oc-mirror-dev@" + digest.String(); string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}

	if _, err := inventory.render("test", []byte(`{{ digest "unknown:tag" }}`)); err == nil {
		t.Error("expected an error for an unknown image")
	}
}
