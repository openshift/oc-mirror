package fixtures

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"text/template"

	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

// RenderBundles replaces the bundle placeholders of the file-based catalog in fbcDir with the
// bundles rendered from their sources, the way `opm render <bundle directory>` renders them.
// Like in the basic catalog template, a placeholder is an olm.bundle with only an image, and
// bundleDir returns the source directory of that image. Other objects are left unchanged.
func RenderBundles(ctx context.Context, fbcDir string, bundleDir func(image string) (string, error)) error {
	return filepath.WalkDir(fbcDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		switch filepath.Ext(path) {
		case ".yaml", ".yml", ".json":
			return renderBundlesInFile(ctx, path, bundleDir)
		}
		return nil
	})
}

func renderBundlesInFile(ctx context.Context, path string, bundleDir func(image string) (string, error)) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var objects []json.RawMessage
	decoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	for {
		var object json.RawMessage
		if err := decoder.Decode(&object); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return fmt.Errorf("failed to parse %s: %w", path, err)
		}
		if len(object) > 0 && string(object) != "null" {
			objects = append(objects, object)
		}
	}

	rendered := false
	for i, object := range objects {
		bundleImage, ok := bundlePlaceholder(object)
		if !ok {
			continue
		}
		dir, err := bundleDir(bundleImage)
		if err != nil {
			return fmt.Errorf("failed to render bundle %s in %s: %w", bundleImage, path, err)
		}
		bundle, err := renderBundle(ctx, dir, bundleImage)
		if err != nil {
			return fmt.Errorf("failed to render bundle %s from %s: %w", bundleImage, dir, err)
		}
		if objects[i], err = json.Marshal(bundle); err != nil {
			return err
		}
		rendered = true
	}
	if !rendered {
		return nil
	}

	var out bytes.Buffer
	for _, object := range objects {
		if filepath.Ext(path) == ".json" {
			if err := json.Indent(&out, object, "", "  "); err != nil {
				return err
			}
			out.WriteString("\n")
			continue
		}
		document, err := yaml.JSONToYAML(object)
		if err != nil {
			return err
		}
		out.WriteString("---\n")
		out.Write(document)
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}

// bundlePlaceholder returns the image of object if it is an olm.bundle with only an image.
func bundlePlaceholder(object json.RawMessage) (string, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(object, &fields); err != nil || len(fields) != 2 {
		return "", false
	}
	var schema, bundleImage string
	if json.Unmarshal(fields["schema"], &schema) != nil || schema != declcfg.SchemaBundle {
		return "", false
	}
	if json.Unmarshal(fields["image"], &bundleImage) != nil || bundleImage == "" {
		return "", false
	}
	return bundleImage, true
}

// renderBundle uses the same action as opm render --alpha-image-ref-template.
func renderBundle(ctx context.Context, dir, bundleImage string) (*declcfg.Bundle, error) {
	imageRef, err := template.New("image-ref").Parse(fmt.Sprintf("{{%q}}", bundleImage))
	if err != nil {
		return nil, err
	}
	render := action.Render{
		Refs:             []string{dir},
		AllowedRefMask:   action.RefBundleDir,
		ImageRefTemplate: imageRef,
	}
	cfg, err := render.Run(ctx)
	if err != nil {
		return nil, err
	}
	if len(cfg.Bundles) != 1 {
		return nil, fmt.Errorf("expected one bundle, got %d", len(cfg.Bundles))
	}
	return &cfg.Bundles[0], nil
}
