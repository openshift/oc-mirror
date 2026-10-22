package testimages

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// Inventory records the digest of every image pushed by BuildAndPush, keyed by
// "<repository>:<tag>" without the registry host
// (e.g. "oc-mirror/oc-mirror-dev:test-catalog-latest").
type Inventory struct {
	digests map[string]v1.Hash
}

func newInventory() *Inventory {
	return &Inventory{digests: map[string]v1.Hash{}}
}

func (i *Inventory) add(repoTag string, digest v1.Hash) {
	i.digests[repoTag] = digest
}

// Digest returns the manifest digest (e.g. "sha256:abcd...") of the test image
// identified by repoTag.
func (i *Inventory) Digest(repoTag string) (string, error) {
	digest, ok := i.digests[repoTag]
	if !ok {
		return "", fmt.Errorf("image %q is not among the test images; known images: %s",
			repoTag, strings.Join(i.repoTags(), ", "))
	}
	return digest.String(), nil
}

// Ref returns the full tag reference of the test image identified by repoTag
// (e.g. "localhost:5001/oc-mirror/oc-mirror-dev:test-catalog-latest").
func (i *Inventory) Ref(repoTag string) (string, error) {
	if _, err := i.Digest(repoTag); err != nil {
		return "", err
	}
	return SourceRegistry + "/" + repoTag, nil
}

func (i *Inventory) repoTags() []string {
	keys := make([]string, 0, len(i.digests))
	for k := range i.digests {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// render executes content as a Go template that can reference test images:
//
//	{{ digest "oc-mirror/oc-mirror-dev:operand" }} -> sha256:...
//
// Content without template actions is returned unchanged.
func (i *Inventory) render(name string, content []byte) ([]byte, error) {
	if !bytes.Contains(content, []byte("{{")) {
		return content, nil
	}
	tmpl, err := template.New(name).
		Option("missingkey=error").
		Funcs(template.FuncMap{"digest": i.Digest}).
		Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parsing template %s: %w", name, err)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, nil); err != nil {
		return nil, fmt.Errorf("rendering template %s: %w", name, err)
	}
	return out.Bytes(), nil
}

// RenderDir copies srcDir to dstDir, rendering each file as a template (see render).
// It lets ImageSetConfigurations reference test images by digest, whose value
// is only known once the images have been built.
func (i *Inventory) RenderDir(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rendered, err := i.render(path, content)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, rendered, 0o644)
	})
}
