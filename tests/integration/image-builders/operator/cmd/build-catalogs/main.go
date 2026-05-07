package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"text/template"

	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/pkg/image"
	"github.com/operator-framework/operator-registry/pkg/registry"
)

const registryNS = "quay.io/oc-mirror/oc-mirror-dev"

var (
	baseDir    string
	bundlesDir string
	outputDir  string
)

func init() {
	_, src, _, _ := runtime.Caller(0)
	baseDir = filepath.Dir(filepath.Dir(filepath.Dir(src)))
	bundlesDir = filepath.Join(baseDir, "bundles")
	outputDir = filepath.Join(baseDir, "_output")
}

var catalogs = map[string]map[string][]string{
	"prune": {
		"foo": {"0.1.0", "0.1.1"},
		"bar": {"0.1.0"},
	},
	"prune-diff": {
		"foo": {"0.2.0"},
		"bar": {"0.1.0"},
	},
	"latest": {
		"foo": {"0.1.0", "0.2.0", "0.3.0", "0.3.1"},
		"bar": {"0.1.0", "0.2.0", "1.0.0"},
		"baz": {"1.0.0", "1.0.1", "1.1.0"},
	},
	"diff": {
		"foo": {"0.1.0", "0.2.0", "0.3.0", "0.3.1", "0.3.2"},
		"bar": {"0.1.0", "0.2.0", "1.0.0"},
		"baz": {"1.0.0", "1.0.1", "1.1.0"},
	},
}

func main() {
	for _, name := range sortedKeys(catalogs) {
		if err := generateCatalog(name, catalogs[name]); err != nil {
			log.Fatalf("catalog %s: %v", name, err)
		}
		fmt.Printf("Generated and validated: %s\n", name)
	}
}

func generateCatalog(name string, pkgs map[string][]string) error {
	ctx := context.Background()
	imgTmpl := template.Must(template.New("img").Parse(
		registryNS + ":{{.Package}}-bundle-v{{.Version}}"))

	var cfg declcfg.DeclarativeConfig

	for _, pkg := range sortedKeys(pkgs) {
		versions := pkgs[pkg]

		var bundles []*registry.Bundle
		for _, ver := range versions {
			dir := bundlePath(pkg, ver)
			b, err := readBundle(dir)
			if err != nil {
				return fmt.Errorf("bundle %s/%s: %w", pkg, ver, err)
			}
			bundles = append(bundles, b)
		}

		lastBundle := bundles[len(bundles)-1]
		p, err := makePackage(pkg, lastBundle.Annotations.DefaultChannelName)
		if err != nil {
			return fmt.Errorf("package %s: %w", pkg, err)
		}
		cfg.Packages = append(cfg.Packages, p)

		for _, ver := range versions {
			dir := bundlePath(pkg, ver)
			render := action.Render{
				Refs:             []string{dir},
				ImageRefTemplate: imgTmpl,
			}
			result, err := render.Run(ctx)
			if err != nil {
				return fmt.Errorf("render %s/%s: %w", pkg, ver, err)
			}
			cfg.Bundles = append(cfg.Bundles, result.Bundles...)
		}

		cfg.Channels = append(cfg.Channels, buildChannels(pkg, bundles)...)
	}

	// Validate
	if _, err := declcfg.ConvertToModel(cfg); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	// Write index.yaml
	catalogDir := filepath.Join(outputDir, "test-catalog-"+name)
	if err := os.MkdirAll(catalogDir, 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(catalogDir, "index.yaml"))
	if err != nil {
		return err
	}
	defer f.Close()
	if err := declcfg.WriteYAML(cfg, f); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	// Generate Dockerfile
	dockerfilePath := filepath.Join(outputDir, "test-catalog-"+name+".Dockerfile")
	df, err := os.Create(dockerfilePath)
	if err != nil {
		return err
	}
	defer df.Close()

	gen := action.GenerateDockerfile{
		BaseImage:    "quay.io/operator-framework/opm:latest",
		BuilderImage: "quay.io/operator-framework/opm:latest",
		IndexDir:     "test-catalog-" + name,
		Writer:       df,
	}
	return gen.Run()
}

func readBundle(bundleDir string) (*registry.Bundle, error) {
	input, err := registry.NewImageInput(image.SimpleReference(""), bundleDir)
	if err != nil {
		return nil, err
	}
	return input.Bundle, nil
}

func makePackage(pkg, defaultChannel string) (declcfg.Package, error) {
	iconData, err := os.ReadFile(filepath.Join(bundlesDir, pkg, pkg+".svg"))
	if err != nil {
		return declcfg.Package{}, fmt.Errorf("read icon: %w", err)
	}
	descData, err := os.ReadFile(filepath.Join(bundlesDir, pkg, "README.md"))
	if err != nil {
		return declcfg.Package{}, fmt.Errorf("read description: %w", err)
	}
	return declcfg.Package{
		Schema:         "olm.package",
		Name:           pkg,
		DefaultChannel: defaultChannel,
		Description:    string(descData),
		Icon: &declcfg.Icon{
			Data:      iconData,
			MediaType: "image/svg+xml",
		},
	}, nil
}

func buildChannels(pkg string, bundles []*registry.Bundle) []declcfg.Channel {
	type channelData struct {
		bundles []*registry.Bundle
	}
	channels := map[string]*channelData{}
	for _, b := range bundles {
		for _, ch := range b.Channels {
			if channels[ch] == nil {
				channels[ch] = &channelData{}
			}
			channels[ch].bundles = append(channels[ch].bundles, b)
		}
	}

	var result []declcfg.Channel
	for _, chName := range sortedKeys(channels) {
		chData := channels[chName]

		inChannel := map[string]bool{}
		for _, b := range chData.bundles {
			inChannel[b.Name] = true
		}

		var entries []declcfg.ChannelEntry
		for _, b := range chData.bundles {
			replaces, _ := b.Replaces()
			skips, _ := b.Skips()
			skipRange, _ := b.SkipRange()

			entry := declcfg.ChannelEntry{
				Name:      b.Name,
				SkipRange: skipRange,
				Skips:     skips,
			}
			if replaces != "" && inChannel[replaces] {
				entry.Replaces = replaces
			}
			entries = append(entries, entry)
		}

		result = append(result, declcfg.Channel{
			Schema:  "olm.channel",
			Package: pkg,
			Name:    chName,
			Entries: entries,
		})
	}
	return result
}

func bundlePath(pkg, ver string) string {
	return filepath.Join(bundlesDir, pkg, fmt.Sprintf("%s-bundle-v%s", pkg, ver))
}

func sortedKeys[M ~map[string]V, V any](m M) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}