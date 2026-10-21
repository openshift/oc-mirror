# Generate Config From Registry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `oc-mirror --v2 generate docker://<registry>` which reverse-engineers an `ImageSetConfiguration` and/or `DeleteImageSetConfiguration` from the content of an existing mirror registry.

**Architecture:** New package `internal/pkg/generate` with a `Source` interface (registry reads), pure builders that turn tags and catalog models into `v2alpha1` types, and a YAML writer. A thin cobra command in `internal/pkg/cli/generate.go` wires `mirror.CopyOptions` auth/TLS into a production `RegistrySource`.

**Tech Stack:** Go 1.26, cobra, `go.podman.io/image/v5` (tags, image inspect, creds), `go-containerregistry` (`remote.Catalog`, plus `pkg/registry` and `pkg/v1/random` in tests; already a direct dependency), `operator-registry` `model.Model`, `sigs.k8s.io/yaml`, testify. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-10-21-generate-config-design.md`

## Global Constraints

- v2 only: work in the repo root, never under `v1/`.
- Build and test with the tags: `json1 exclude_graphdriver_devicemapper exclude_graphdriver_btrfs containers_image_openpgp`. Use `make build`, never plain `go build`.
- Single-package test command: `go test -tags "json1 exclude_graphdriver_devicemapper exclude_graphdriver_btrfs containers_image_openpgp" -short -race -count=1 ./internal/pkg/generate/...`
- No new dependencies (CLAUDE.md rule 13).
- Do not modify or delete existing tests.
- American English, no em dashes in any text written.
- Commit messages end with `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`.
- Run `make sanity` before the final commit.
- Push only to remote `ammachado` (`https://github.com/ammachado/oc-mirror.git`), only when everything is done.

## Review Focus

- Registry with `/v2/_catalog` disabled and no `--repos`: fail with an actionable hint, not an empty config (Task 5, 6).
- Mirror holds thousands of digest-tagged images (operator related images, release components, Helm images): they must not flood `additionalImages` (Task 3).
- Signature/attestation tags (`.sig`, `.att`, `.sbom`) must never become entries (Task 1, 3).
- Pre-release release tags such as `4.16.0-rc.1-x86_64` must parse and order correctly (Task 1).
- Output file already exists: refuse to overwrite the user's real config (Task 4).
- Empty registry: write valid, parseable YAML or fail clearly, never a file that `ReadConfig` rejects (Task 4).

## Known deviation from the spec (flagged for the user)

The spec says "all remaining repos and tags" become `additionalImages`. A registry populated by oc-mirror also holds digest-mirrored images (12 hex char tags, `sha256-<digest>` tags) that belong to operators, releases, or Helm charts and cannot be told apart from digest-mirrored additional images. This plan skips those tags and logs a count. Tagged additional images are kept.

## File Structure

- Create `internal/pkg/generate/types.go`: `Source`, `Content`.
- Create `internal/pkg/generate/release.go`: release-images tag parsing into `v2alpha1.Platform`.
- Create `internal/pkg/generate/operators.go`: catalog `model.Model` into `v2alpha1.Operator`.
- Create `internal/pkg/generate/additional.go`: repo/tags into `[]v2alpha1.AdditionalImage`.
- Create `internal/pkg/generate/generate.go`: `Generator.Discover` orchestration.
- Create `internal/pkg/generate/output.go`: ISC/DISC build, YAML marshal, file writing.
- Create `internal/pkg/generate/source.go`: production `RegistrySource`.
- Create `internal/pkg/cli/generate.go` (+ `generate_test.go`): cobra command.
- Modify `internal/pkg/cli/list/operators.go`: add exported `DownloadCatalog` wrapper.
- Modify `internal/pkg/cli/executor.go:249`: register command.
- Create `docs/features/generate-config.md`.

---

### Task 1: Release classifier

**Files:**
- Create: `internal/pkg/generate/types.go`
- Create: `internal/pkg/generate/release.go`
- Test: `internal/pkg/generate/release_test.go`

**Interfaces:**
- Produces: `buildPlatform(tags []string) (v2alpha1.Platform, []string)` (platform, warnings); `isRepo(repo, base string) bool`; consts `releaseImagesRepo`, `releaseContentRepo`; `isSignatureTag(tag string) bool`.

- [ ] **Step 1: Write the failing test**

```go
package generate

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

func TestBuildPlatform(t *testing.T) {
	tags := []string{
		"4.16.1-x86_64", "4.16.3-x86_64", "4.16.2-aarch64",
		"4.17.0-rc.1-x86_64", "4.17.0-x86_64",
		"4.16.3-x86_64.sig", "sha256-abcd.sig", "latest",
	}
	p, warns := buildPlatform(tags)

	assert.Equal(t, []string{"amd64", "arm64"}, p.Architectures)
	assert.Equal(t, []v2alpha1.ReleaseChannel{
		{Name: "stable-4.16", Type: v2alpha1.TypeOCP, MinVersion: "4.16.1", MaxVersion: "4.16.3"},
		{Name: "stable-4.17", Type: v2alpha1.TypeOCP, MinVersion: "4.17.0-rc.1", MaxVersion: "4.17.0"},
	}, p.Channels)
	assert.NotEmpty(t, warns, "derived channels must be reported as inferred")
}

func TestBuildPlatformNoReleaseTags(t *testing.T) {
	p, warns := buildPlatform([]string{"latest", "foo.sig"})
	assert.Empty(t, p.Channels)
	assert.Empty(t, warns)
}

func TestIsRepo(t *testing.T) {
	assert.True(t, isRepo("openshift/release", releaseContentRepo))
	assert.True(t, isRepo("mirror/openshift/release", releaseContentRepo))
	assert.False(t, isRepo("openshift/release-images", releaseContentRepo))
	assert.False(t, isRepo("xopenshift/release", releaseContentRepo))
}
```

- [ ] **Step 2: Run to verify it fails** (`undefined: buildPlatform`)

Run the single-package test command from Global Constraints.

- [ ] **Step 3: Implement**

`types.go`:

```go
// Package generate reverse-engineers ImageSetConfiguration and
// DeleteImageSetConfiguration files from the content of a mirror registry.
package generate

import (
	"context"

	"github.com/operator-framework/operator-registry/alpha/model"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

// Source reads the content of a registry. Repository names are relative to
// the registry host (for example "openshift/release-images").
type Source interface {
	Repositories(ctx context.Context) ([]string, error)
	Tags(ctx context.Context, repo string) ([]string, error)
	// IsCatalog reports whether repo:tag is an operator catalog image.
	IsCatalog(ctx context.Context, repo, tag string) (bool, error)
	CatalogModel(ctx context.Context, repo, tag string) (model.Model, error)
}

// Content is what was found in a registry, independent of the output kind.
type Content struct {
	Platform         v2alpha1.Platform
	Operators        []v2alpha1.Operator
	AdditionalImages []v2alpha1.AdditionalImage
	// Warnings describe lossy inference the user should review.
	Warnings []string
}
```

`release.go`:

```go
package generate

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/blang/semver/v4"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

const (
	releaseImagesRepo  = "openshift/release-images"
	releaseContentRepo = "openshift/release"
)

// Tag format: <version>-<arch>, for example 4.16.3-x86_64 or 4.17.0-rc.1-multi.
var releaseTagRe = regexp.MustCompile(`^(\d+\.\d+\.\d+.*)-(x86_64|aarch64|ppc64le|s390x|multi)$`)

var archNames = map[string]string{
	"x86_64":  "amd64",
	"aarch64": "arm64",
	"ppc64le": "ppc64le",
	"s390x":   "s390x",
	"multi":   "multi",
}

// isRepo reports whether repo is base, optionally under a registry path prefix.
func isRepo(repo, base string) bool {
	return repo == base || strings.HasSuffix(repo, "/"+base)
}

func isSignatureTag(tag string) bool {
	return strings.HasSuffix(tag, ".sig") || strings.HasSuffix(tag, ".att") || strings.HasSuffix(tag, ".sbom")
}

// buildPlatform infers platform channels and architectures from the tags of
// the release-images repository. Channels are derived as stable-X.Y; the
// original channel names cannot be recovered from a registry.
func buildPlatform(tags []string) (v2alpha1.Platform, []string) {
	type span struct{ min, max semver.Version }
	spans := map[string]*span{}
	archs := map[string]struct{}{}
	var warns []string

	for _, tag := range tags {
		if isSignatureTag(tag) {
			continue
		}
		m := releaseTagRe.FindStringSubmatch(tag)
		if m == nil {
			continue
		}
		v, err := semver.Parse(m[1])
		if err != nil {
			warns = append(warns, fmt.Sprintf("release tag %q skipped: %v", tag, err))
			continue
		}
		archs[archNames[m[2]]] = struct{}{}
		key := fmt.Sprintf("%d.%d", v.Major, v.Minor)
		s, ok := spans[key]
		if !ok {
			spans[key] = &span{min: v, max: v}
			continue
		}
		if v.LT(s.min) {
			s.min = v
		}
		if v.GT(s.max) {
			s.max = v
		}
	}

	var p v2alpha1.Platform
	if len(spans) == 0 {
		return p, warns
	}
	for _, key := range slices.Sorted(mapKeys(spans)) {
		p.Channels = append(p.Channels, v2alpha1.ReleaseChannel{
			Name:       "stable-" + key,
			Type:       v2alpha1.TypeOCP,
			MinVersion: spans[key].min.String(),
			MaxVersion: spans[key].max.String(),
		})
	}
	p.Architectures = slices.Sorted(mapKeys(archs)) //nolint:staticcheck // SA1019: matches how existing configs declare architectures
	sort.Strings(p.Architectures)
	warns = append(warns, "platform channels are inferred as stable-X.Y with min/max versions from release tags; review before use")
	return p, warns
}
```

In the code above, import `"maps"`, replace every `mapKeys(` with `maps.Keys(`, and drop the `sort` import and the `sort.Strings(p.Architectures)` line (`slices.Sorted(maps.Keys(m))` already sorts).

- [ ] **Step 4: Run test, expect PASS**
- [ ] **Step 5: Commit** `feat(generate): infer platform channels from release tags`

---

### Task 2: Operator catalog builder

**Files:**
- Create: `internal/pkg/generate/operators.go`
- Test: `internal/pkg/generate/operators_test.go`

**Interfaces:**
- Produces: `buildOperator(ref string, m model.Model) v2alpha1.Operator`.

- [ ] **Step 1: Failing test**

```go
package generate

import (
	"testing"

	"github.com/blang/semver/v4"
	"github.com/operator-framework/operator-registry/alpha/model"
	"github.com/stretchr/testify/assert"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

func TestBuildOperator(t *testing.T) {
	pkg := &model.Package{Name: "foo"}
	pkg.Channels = map[string]*model.Channel{
		"stable": {Name: "stable", Package: pkg, Bundles: map[string]*model.Bundle{
			"foo.v1.0.0": {Name: "foo.v1.0.0", Version: semver.MustParse("1.0.0")},
			"foo.v1.2.0": {Name: "foo.v1.2.0", Version: semver.MustParse("1.2.0")},
			"foo.v1.1.0": {Name: "foo.v1.1.0", Version: semver.MustParse("1.1.0")},
		}},
		"empty": {Name: "empty", Package: pkg, Bundles: map[string]*model.Bundle{}},
	}
	op := buildOperator("reg.example.com/redhat/redhat-operator-index:v4.16", model.Model{"foo": pkg})

	assert.Equal(t, "reg.example.com/redhat/redhat-operator-index:v4.16", op.Catalog)
	assert.True(t, op.Full)
	assert.Equal(t, []v2alpha1.IncludePackage{{
		Name: "foo",
		Channels: []v2alpha1.IncludeChannel{
			{Name: "empty"},
			{Name: "stable", IncludeBundle: v2alpha1.IncludeBundle{MinVersion: "1.0.0", MaxVersion: "1.2.0"}},
		},
	}}, op.Packages)
}
```

- [ ] **Step 2: Run, expect FAIL** (`undefined: buildOperator`)
- [ ] **Step 3: Implement**

```go
package generate

import (
	"maps"
	"slices"

	"github.com/blang/semver/v4"
	"github.com/operator-framework/operator-registry/alpha/model"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

// buildOperator describes a mirrored (already filtered) catalog. Full is set
// because the catalog in the mirror only holds what was mirrored, so each
// package and channel is included whole, with version bounds read from its bundles.
func buildOperator(ref string, m model.Model) v2alpha1.Operator {
	op := v2alpha1.Operator{Catalog: ref, Full: true}
	for _, name := range slices.Sorted(maps.Keys(m)) {
		pkg := m[name]
		ip := v2alpha1.IncludePackage{Name: name}
		for _, chName := range slices.Sorted(maps.Keys(pkg.Channels)) {
			ic := v2alpha1.IncludeChannel{Name: chName}
			var lo, hi *semver.Version
			for _, b := range pkg.Channels[chName].Bundles {
				v := b.Version
				if lo == nil || v.LT(*lo) {
					lo = &v
				}
				if hi == nil || v.GT(*hi) {
					hi = &v
				}
			}
			if lo != nil {
				ic.MinVersion, ic.MaxVersion = lo.String(), hi.String()
			}
			ip.Channels = append(ip.Channels, ic)
		}
		op.Packages = append(op.Packages, ip)
	}
	return op
}
```

- [ ] **Step 4: Run, expect PASS**
- [ ] **Step 5: Commit** `feat(generate): build operator config from catalog model`

---

### Task 3: Additional images

**Files:**
- Create: `internal/pkg/generate/additional.go`
- Test: `internal/pkg/generate/additional_test.go`

**Interfaces:**
- Produces: `buildAdditional(host, repo string, tags []string) (images []v2alpha1.AdditionalImage, skipped int)`.
- Consumes: `isSignatureTag` (Task 1).

- [ ] **Step 1: Failing test**

```go
package generate

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

func TestBuildAdditional(t *testing.T) {
	imgs, skipped := buildAdditional("reg.example.com", "ubi9/ubi", []string{
		"latest", "9.4",
		"0123456789ab",             // digest-derived tag
		"sha256-" + hex64() + ".sig", // signature
		"sha256-" + hex64(),        // digest-derived tag
	})
	assert.Equal(t, []v2alpha1.AdditionalImage{
		{Name: "reg.example.com/ubi9/ubi:9.4"},
		{Name: "reg.example.com/ubi9/ubi:latest"},
	}, imgs)
	assert.Equal(t, 2, skipped, "digest-derived tags counted, signatures not")
}

func hex64() string {
	s := ""
	for len(s) < 64 {
		s += "ab"
	}
	return s[:64]
}
```

- [ ] **Step 2: Run, expect FAIL**
- [ ] **Step 3: Implement**

```go
package generate

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

// Tags oc-mirror gives to images it mirrors by digest: 12 hex characters or
// sha256-<digest>. They belong to operators, releases or Helm charts and are
// indistinguishable from digest-mirrored additional images, so they are skipped.
var digestTagRe = regexp.MustCompile(`^([a-f0-9]{12}|sha256-[a-f0-9]{64})$`)

func buildAdditional(host, repo string, tags []string) ([]v2alpha1.AdditionalImage, int) {
	var imgs []v2alpha1.AdditionalImage
	skipped := 0
	for _, tag := range slices.Sorted(slices.Values(tags)) {
		if isSignatureTag(tag) {
			continue
		}
		if digestTagRe.MatchString(tag) {
			skipped++
			continue
		}
		imgs = append(imgs, v2alpha1.AdditionalImage{Name: fmt.Sprintf("%s/%s:%s", host, repo, tag)})
	}
	return imgs, skipped
}
```

- [ ] **Step 4: Run, expect PASS**
- [ ] **Step 5: Commit** `feat(generate): collect tagged additional images`

---

### Task 4: Discovery and output

**Files:**
- Create: `internal/pkg/generate/generate.go`, `internal/pkg/generate/output.go`
- Test: `internal/pkg/generate/generate_test.go`, `internal/pkg/generate/output_test.go`

**Interfaces:**
- Consumes: Tasks 1-3, `Source`, `Content`.
- Produces:
  - `type Generator struct { Source Source; Host string; Prefix string }`
  - `func (g *Generator) Discover(ctx context.Context) (Content, error)`
  - `type OutputType string`; consts `OutputISC = "isc"`, `OutputDISC = "disc"`, `OutputBoth = "both"`
  - `func WriteFiles(dir string, c Content, t OutputType) ([]string, error)` (returns written paths)
  - `const ISCFileName = "imageset-config.yaml"`, `DISCFileName = "delete-imageset-config.yaml"`

- [ ] **Step 1: Failing tests**

`generate_test.go` (fake source):

```go
package generate

import (
	"context"
	"errors"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/operator-framework/operator-registry/alpha/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSource struct {
	tags     map[string][]string
	catalogs map[string]model.Model // key repo:tag
	repoErr  error
}

func (f *fakeSource) Repositories(context.Context) ([]string, error) {
	if f.repoErr != nil {
		return nil, f.repoErr
	}
	var r []string
	for k := range f.tags {
		r = append(r, k)
	}
	return r, nil
}
func (f *fakeSource) Tags(_ context.Context, repo string) ([]string, error) { return f.tags[repo], nil }
func (f *fakeSource) IsCatalog(_ context.Context, repo, tag string) (bool, error) {
	_, ok := f.catalogs[repo+":"+tag]
	return ok, nil
}
func (f *fakeSource) CatalogModel(_ context.Context, repo, tag string) (model.Model, error) {
	return f.catalogs[repo+":"+tag], nil
}

func TestDiscover(t *testing.T) {
	pkg := &model.Package{Name: "foo"}
	pkg.Channels = map[string]*model.Channel{"stable": {Name: "stable", Package: pkg,
		Bundles: map[string]*model.Bundle{"foo.v1": {Name: "foo.v1", Version: semver.MustParse("1.0.0")}}}}
	src := &fakeSource{
		tags: map[string][]string{
			"openshift/release-images":      {"4.16.3-x86_64"},
			"openshift/release":             {"4.16.3-x86_64-etcd"},
			"redhat/redhat-operator-index":  {"v4.16"},
			"ubi9/ubi":                      {"latest", "0123456789ab"},
		},
		catalogs: map[string]model.Model{"redhat/redhat-operator-index:v4.16": {"foo": pkg}},
	}
	c, err := (&Generator{Source: src, Host: "reg.example.com"}).Discover(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "stable-4.16", c.Platform.Channels[0].Name)
	require.Len(t, c.Operators, 1)
	assert.Equal(t, "reg.example.com/redhat/redhat-operator-index:v4.16", c.Operators[0].Catalog)
	require.Len(t, c.AdditionalImages, 1, "release content and catalog repos must not become additional images")
	assert.Equal(t, "reg.example.com/ubi9/ubi:latest", c.AdditionalImages[0].Name)
	assert.NotEmpty(t, c.Warnings)
}

func TestDiscoverPrefix(t *testing.T) {
	src := &fakeSource{tags: map[string][]string{"team-a/app": {"1"}, "team-b/app": {"1"}}}
	c, err := (&Generator{Source: src, Host: "h", Prefix: "team-a/"}).Discover(context.Background())
	require.NoError(t, err)
	require.Len(t, c.AdditionalImages, 1)
	assert.Equal(t, "h/team-a/app:1", c.AdditionalImages[0].Name)
}

func TestDiscoverRepoError(t *testing.T) {
	src := &fakeSource{repoErr: errors.New("boom")}
	_, err := (&Generator{Source: src, Host: "h"}).Discover(context.Background())
	assert.ErrorContains(t, err, "boom")
}
```

`output_test.go` (round trip through existing config loader, proves generated YAML is valid):

```go
package generate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/config"
)

func sampleContent() Content {
	return Content{
		Platform: v2alpha1.Platform{
			Channels:      []v2alpha1.ReleaseChannel{{Name: "stable-4.16", Type: v2alpha1.TypeOCP, MinVersion: "4.16.1", MaxVersion: "4.16.3"}},
			Architectures: []string{"amd64"},
		},
		Operators: []v2alpha1.Operator{{
			Catalog:       "reg.example.com/redhat/redhat-operator-index:v4.16",
			Full:          true,
			IncludeConfig: v2alpha1.IncludeConfig{Packages: []v2alpha1.IncludePackage{{Name: "foo"}}},
		}},
		AdditionalImages: []v2alpha1.AdditionalImage{{Name: "reg.example.com/ubi9/ubi:latest"}},
	}
}

func TestWriteFilesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	paths, err := WriteFiles(dir, sampleContent(), OutputBoth)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(dir, ISCFileName), filepath.Join(dir, DISCFileName)}, paths)

	isc, err := config.ReadConfig(paths[0], v2alpha1.ImageSetConfigurationKind)
	require.NoError(t, err)
	assert.Equal(t, "reg.example.com/ubi9/ubi:latest",
		isc.(v2alpha1.ImageSetConfiguration).Mirror.AdditionalImages[0].Name)

	disc, err := config.ReadConfig(paths[1], v2alpha1.DeleteImageSetConfigurationKind)
	require.NoError(t, err)
	assert.Equal(t, "stable-4.16",
		disc.(v2alpha1.DeleteImageSetConfiguration).Delete.Platform.Channels[0].Name)
}

func TestWriteFilesRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, ISCFileName)
	require.NoError(t, os.WriteFile(existing, []byte("mine"), 0o600))

	_, err := WriteFiles(dir, sampleContent(), OutputISC)
	assert.ErrorContains(t, err, "already exists")
	got, _ := os.ReadFile(existing)
	assert.Equal(t, "mine", string(got), "user file must be untouched")
}

func TestWriteFilesEmptyContentStillParses(t *testing.T) {
	dir := t.TempDir()
	paths, err := WriteFiles(dir, Content{}, OutputISC)
	require.NoError(t, err)
	_, err = config.ReadConfig(paths[0], v2alpha1.ImageSetConfigurationKind)
	assert.NoError(t, err)
}
```

If `ReadConfig` returns the config by value vs pointer differently than asserted, adjust the type assertion to what `config.ReadConfig` returns (it returns `cfg` of type `v2alpha1.ImageSetConfiguration`, a value). If the empty-content test shows the `mirror` key missing, fix by ensuring `Mirror` serializes (see Step 3 note).

- [ ] **Step 2: Run, expect FAIL**
- [ ] **Step 3: Implement**

`generate.go`:

```go
package generate

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// Generator discovers what a registry holds.
type Generator struct {
	Source Source
	// Host is the registry host[:port], used to build image references.
	Host string
	// Prefix, when set, restricts discovery to repositories starting with it.
	Prefix string
}

// Discover classifies every repository as release, operator catalog or additional image.
func (g *Generator) Discover(ctx context.Context) (Content, error) {
	var c Content
	repos, err := g.Source.Repositories(ctx)
	if err != nil {
		return c, err
	}
	slices.Sort(repos)

	var releaseTags []string
	skippedDigests := 0
	for _, repo := range repos {
		if !strings.HasPrefix(repo, g.Prefix) {
			continue
		}
		tags, err := g.Source.Tags(ctx, repo)
		if err != nil {
			return c, fmt.Errorf("listing tags for %q: %w", repo, err)
		}
		switch {
		case isRepo(repo, releaseContentRepo):
			// release payload components are implied by release-images
		case isRepo(repo, releaseImagesRepo):
			releaseTags = append(releaseTags, tags...)
		default:
			if err := g.classify(ctx, &c, repo, tags, &skippedDigests); err != nil {
				return c, err
			}
		}
	}

	var warns []string
	c.Platform, warns = buildPlatform(releaseTags)
	c.Warnings = append(c.Warnings, warns...)
	if skippedDigests > 0 {
		c.Warnings = append(c.Warnings, fmt.Sprintf(
			"%d digest-derived tags skipped (operator related images, release components, Helm images); they cannot be told apart from digest-mirrored additional images", skippedDigests))
	}
	c.Warnings = append(c.Warnings, "image references point at the mirror registry, not the original sources")
	return c, nil
}

func (g *Generator) classify(ctx context.Context, c *Content, repo string, tags []string, skipped *int) error {
	var candidates []string
	for _, t := range slices.Sorted(slices.Values(tags)) {
		if !isSignatureTag(t) {
			candidates = append(candidates, t)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	isCatalog, err := g.Source.IsCatalog(ctx, repo, candidates[0])
	if err != nil {
		return fmt.Errorf("inspecting %s:%s: %w", repo, candidates[0], err)
	}
	if !isCatalog {
		imgs, n := buildAdditional(g.Host, repo, candidates)
		c.AdditionalImages = append(c.AdditionalImages, imgs...)
		*skipped += n
		return nil
	}
	for _, tag := range candidates {
		m, err := g.Source.CatalogModel(ctx, repo, tag)
		if err != nil {
			return fmt.Errorf("reading catalog %s:%s: %w", repo, tag, err)
		}
		c.Operators = append(c.Operators, buildOperator(fmt.Sprintf("%s/%s:%s", g.Host, repo, tag), m))
	}
	return nil
}
```

`output.go`:

```go
package generate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

type OutputType string

const (
	OutputISC  OutputType = "isc"
	OutputDISC OutputType = "disc"
	OutputBoth OutputType = "both"

	ISCFileName  = "imageset-config.yaml"
	DISCFileName = "delete-imageset-config.yaml"
)

func (c Content) imageSetConfig() v2alpha1.ImageSetConfiguration {
	cfg := v2alpha1.ImageSetConfiguration{ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
		Mirror: v2alpha1.Mirror{Platform: c.Platform, Operators: c.Operators, AdditionalImages: c.AdditionalImages},
	}}
	cfg.SetGroupVersionKind(v2alpha1.GroupVersion.WithKind(v2alpha1.ImageSetConfigurationKind))
	return cfg
}

func (c Content) deleteImageSetConfig() v2alpha1.DeleteImageSetConfiguration {
	cfg := v2alpha1.DeleteImageSetConfiguration{DeleteImageSetConfigurationSpec: v2alpha1.DeleteImageSetConfigurationSpec{
		Delete: v2alpha1.Delete{Platform: c.Platform, Operators: c.Operators, AdditionalImages: c.AdditionalImages},
	}}
	cfg.SetGroupVersionKind(v2alpha1.GroupVersion.WithKind(v2alpha1.DeleteImageSetConfigurationKind))
	return cfg
}

func header(warnings []string) string {
	var b strings.Builder
	b.WriteString("# Generated by oc-mirror generate. Review before use.\n")
	for _, w := range warnings {
		fmt.Fprintf(&b, "# NOTE: %s\n", w)
	}
	return b.String()
}

func writeNew(path, hdr string, obj any) error {
	body, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s already exists; move it or use another --output-dir", path)
		}
		return err
	}
	defer f.Close()
	_, err = f.WriteString(hdr + string(body))
	return err
}

// WriteFiles writes the requested config files into dir and returns their
// paths. Existing files are never overwritten.
func WriteFiles(dir string, c Content, t OutputType) ([]string, error) {
	var paths []string
	hdr := header(c.Warnings)
	if t == OutputISC || t == OutputBoth {
		p := filepath.Join(dir, ISCFileName)
		if err := writeNew(p, hdr, c.imageSetConfig()); err != nil {
			return paths, err
		}
		paths = append(paths, p)
	}
	if t == OutputDISC || t == OutputBoth {
		p := filepath.Join(dir, DISCFileName)
		if err := writeNew(p, hdr, c.deleteImageSetConfig()); err != nil {
			return paths, err
		}
		paths = append(paths, p)
	}
	return paths, nil
}
```

Note: with `OutputBoth`, if the DISC file exists the ISC file was already written. Pre-check both paths for existence before writing anything so a failure leaves no partial output; add that loop at the top of `WriteFiles` (stat each target, return the same "already exists" error) and extend `TestWriteFilesRefusesOverwrite` with an `OutputBoth` case asserting the ISC file was not created.

If `v2alpha1.Mirror` serializes without a `mirror:` key for empty content (it is `json:"mirror"` in the spec struct; confirm in `type_config.go`), the empty-content test fails: in that case make `imageSetConfig` leave Mirror as is and assert on the real behavior, since `ReadConfig` requires a `mirror` key.

- [ ] **Step 4: Run tests, expect PASS**
- [ ] **Step 5: Commit** `feat(generate): discover registry content and write ISC/DISC`

---

### Task 5: Production RegistrySource

**Files:**
- Create: `internal/pkg/generate/source.go`
- Test: `internal/pkg/generate/source_test.go`

**Interfaces:**
- Produces:
  - `type CatalogLoader func(ctx context.Context, catalogRef string) (model.Model, error)`
  - `type RegistrySource struct { Host string; Repos []string; SysCtx *types.SystemContext; LoadCatalog CatalogLoader }` implementing `Source`.
  - `const catalogLabel = "operators.operatorframework.io.index.configs.v1"`

- [ ] **Step 1: Failing test** (in-process registry from `go-containerregistry`)

```go
package generate

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/types"
)

func newTestRegistry(t *testing.T) (host string) {
	t.Helper()
	srv := httptest.NewServer(registry.New())
	t.Cleanup(srv.Close)
	host = strings.TrimPrefix(srv.URL, "http://")

	push := func(ref string, labels map[string]string) {
		img, err := random.Image(64, 1)
		require.NoError(t, err)
		if labels != nil {
			cfg, err := img.ConfigFile()
			require.NoError(t, err)
			cfg.Config.Labels = labels
			img, err = mutate.ConfigFile(img, cfg)
			require.NoError(t, err)
		}
		r, err := name.ParseReference(host+"/"+ref, name.Insecure)
		require.NoError(t, err)
		require.NoError(t, remote.Write(r, img))
	}
	push("redhat/redhat-operator-index:v4.16", map[string]string{catalogLabel: "/configs"})
	push("ubi9/ubi:latest", nil)
	return host
}

func testSysCtx() *types.SystemContext {
	return &types.SystemContext{DockerInsecureSkipTLSVerify: types.OptionalBoolTrue}
}

func TestRegistrySource(t *testing.T) {
	host := newTestRegistry(t)
	s := &RegistrySource{Host: host, SysCtx: testSysCtx()}
	ctx := context.Background()

	repos, err := s.Repositories(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"redhat/redhat-operator-index", "ubi9/ubi"}, repos)

	tags, err := s.Tags(ctx, "ubi9/ubi")
	require.NoError(t, err)
	assert.Equal(t, []string{"latest"}, tags)

	ok, err := s.IsCatalog(ctx, "redhat/redhat-operator-index", "v4.16")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = s.IsCatalog(ctx, "ubi9/ubi", "latest")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRegistrySourceExplicitRepos(t *testing.T) {
	s := &RegistrySource{Host: "unused", Repos: []string{"a/b"}}
	repos, err := s.Repositories(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"a/b"}, repos)
}

func TestRegistrySourceCatalogDisabledHint(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	s := &RegistrySource{Host: strings.TrimPrefix(srv.URL, "http://"), SysCtx: testSysCtx()}
	_, err := s.Repositories(context.Background())
	assert.ErrorContains(t, err, "--repos")
}
```

Add `"net/http"` to the imports of the test file.

- [ ] **Step 2: Run, expect FAIL**
- [ ] **Step 3: Implement**

```go
package generate

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/operator-framework/operator-registry/alpha/model"
	"go.podman.io/image/v5/docker"
	dockerconfig "go.podman.io/image/v5/pkg/docker/config"
	"go.podman.io/image/v5/types"
)

// catalogLabel is set by OLM on operator catalog (file-based catalog) images.
const catalogLabel = "operators.operatorframework.io.index.configs.v1"

// CatalogLoader loads the declarative config of a catalog image reference.
type CatalogLoader func(ctx context.Context, catalogRef string) (model.Model, error)

// RegistrySource reads a live registry. Auth and TLS settings come from SysCtx,
// the same system context oc-mirror uses for source registries.
type RegistrySource struct {
	Host string
	// Repos, when set, replaces /v2/_catalog enumeration.
	Repos       []string
	SysCtx      *types.SystemContext
	LoadCatalog CatalogLoader
}

func (s *RegistrySource) insecure() bool {
	return s.SysCtx != nil && s.SysCtx.DockerInsecureSkipTLSVerify == types.OptionalBoolTrue
}

func (s *RegistrySource) Repositories(ctx context.Context) ([]string, error) {
	if len(s.Repos) > 0 {
		return s.Repos, nil
	}
	var nameOpts []name.Option
	if s.insecure() {
		nameOpts = append(nameOpts, name.Insecure)
	}
	reg, err := name.NewRegistry(s.Host, nameOpts...)
	if err != nil {
		return nil, fmt.Errorf("parsing registry %q: %w", s.Host, err)
	}
	opts := []remote.Option{remote.WithContext(ctx), remote.WithAuth(s.auth())}
	if s.insecure() {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // user passed --src-tls-verify=false
		opts = append(opts, remote.WithTransport(tr))
	}
	repos, err := remote.Catalog(ctx, reg, opts...)
	if err != nil {
		return nil, fmt.Errorf("listing repositories on %s (is /v2/_catalog enabled? otherwise pass --repos): %w", s.Host, err)
	}
	return repos, nil
}

func (s *RegistrySource) auth() authn.Authenticator {
	creds, err := dockerconfig.GetCredentials(s.SysCtx, s.Host)
	if err != nil || creds == (types.DockerAuthConfig{}) {
		return authn.Anonymous
	}
	return authn.FromConfig(authn.AuthConfig{
		Username: creds.Username, Password: creds.Password, IdentityToken: creds.IdentityToken,
	})
}

func (s *RegistrySource) Tags(ctx context.Context, repo string) ([]string, error) {
	ref, err := docker.ParseReference(fmt.Sprintf("//%s/%s", s.Host, repo))
	if err != nil {
		return nil, err
	}
	return docker.GetRepositoryTags(ctx, s.SysCtx, ref)
}

func (s *RegistrySource) IsCatalog(ctx context.Context, repo, tag string) (bool, error) {
	ref, err := docker.ParseReference(fmt.Sprintf("//%s/%s:%s", s.Host, repo, tag))
	if err != nil {
		return false, err
	}
	img, err := ref.NewImage(ctx, s.SysCtx)
	if err != nil {
		return false, err
	}
	defer img.Close()
	info, err := img.Inspect(ctx)
	if err != nil {
		return false, err
	}
	_, ok := info.Labels[catalogLabel]
	return ok, nil
}

func (s *RegistrySource) CatalogModel(ctx context.Context, repo, tag string) (model.Model, error) {
	return s.LoadCatalog(ctx, fmt.Sprintf("%s/%s:%s", s.Host, repo, tag))
}
```

If `dockerconfig.GetCredentials` has a different signature in `go.podman.io/image/v5` v5.41.1 (check with `go doc go.podman.io/image/v5/pkg/docker/config GetCredentials`), adapt the call; behavior must stay "anonymous when no creds".

- [ ] **Step 4: Run tests, expect PASS.** Run `go mod tidy` and confirm `go.mod` is unchanged except possibly moving an indirect to direct; if a new module is added, stop and ask (rule 13).
- [ ] **Step 5: Commit** `feat(generate): read tags, catalog list and catalog label from a registry`

---

### Task 6: CLI command, wiring, docs

**Files:**
- Modify: `internal/pkg/cli/list/operators.go` (add exported wrapper after `downloadCatalog`)
- Create: `internal/pkg/cli/generate.go`, `internal/pkg/cli/generate_test.go`
- Modify: `internal/pkg/cli/executor.go:249` (add `cmd.AddCommand(NewGenerateCommand(log, opts))`)
- Create: `docs/features/generate-config.md`

**Interfaces:**
- Consumes: `generate.RegistrySource`, `generate.Generator`, `generate.WriteFiles`, `generate.OutputType`.
- Produces: `NewGenerateCommand(log clog.PluggableLoggerInterface, opts *mirror.CopyOptions) *cobra.Command`; `(o *generateOptions) validate(args []string) (host string, err error)`.

- [ ] **Step 1: Failing test** (`generate_test.go`, package `cli`)

```go
package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateValidate(t *testing.T) {
	tests := []struct {
		name    string
		opts    generateOptions
		args    []string
		host    string
		wantErr string
	}{
		{name: "ok", opts: generateOptions{outputType: "both"}, args: []string{"docker://reg.example.com:5000"}, host: "reg.example.com:5000"},
		{name: "missing arg", opts: generateOptions{outputType: "both"}, wantErr: "registry argument is required"},
		{name: "no protocol", opts: generateOptions{outputType: "both"}, args: []string{"reg.example.com"}, wantErr: "docker://"},
		{name: "path in arg", opts: generateOptions{outputType: "both"}, args: []string{"docker://reg.example.com/team"}, wantErr: "--prefix"},
		{name: "bad type", opts: generateOptions{outputType: "xml"}, args: []string{"docker://r"}, wantErr: "--output-type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, err := tt.opts.validate(tt.args)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.host, host)
		})
	}
}
```

- [ ] **Step 2: Run** (`go test ... ./internal/pkg/cli/ -run TestGenerateValidate`), expect FAIL.
- [ ] **Step 3: Implement**

`list/operators.go`, below `downloadCatalog`:

```go
// DownloadCatalog loads the declarative config of the catalog image catalog.
func DownloadCatalog(ctx context.Context, log log.PluggableLoggerInterface, catalog string, opts mirror.CopyOptions) (model.Model, error) {
	return downloadCatalog(ctx, log, catalog, opts)
}
```

`cli/generate.go`:

```go
package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/operator-framework/operator-registry/alpha/model"
	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/openshift/oc-mirror/v2/internal/pkg/cli/list"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/generate"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type generateOptions struct {
	outputType string
	outputDir  string
	prefix     string
	repos      []string
	globalOpts *mirror.CopyOptions
}

// NewGenerateCommand returns the `generate` command, which reverse-engineers
// ImageSetConfiguration and DeleteImageSetConfiguration files from a mirror registry.
func NewGenerateCommand(log clog.PluggableLoggerInterface, globalOpts *mirror.CopyOptions) *cobra.Command {
	o := &generateOptions{globalOpts: globalOpts}
	cmd := &cobra.Command{
		Use:   "generate docker://<registry>",
		Short: "Generate ImageSetConfiguration and DeleteImageSetConfiguration files from the content of a mirror registry",
		Example: templates.Examples(`
			# Generate both files in the current directory
			oc-mirror --v2 generate docker://registry.example.com:5000

			# Only a DeleteImageSetConfiguration, restricted to a registry path
			oc-mirror --v2 generate docker://registry.example.com:5000 --prefix=mirror/ --output-type=disc --output-dir=./out

			# Registry with /v2/_catalog disabled
			oc-mirror --v2 generate docker://registry.example.com:5000 --repos=openshift/release-images,redhat/redhat-operator-index
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			host, err := o.validate(args)
			if err != nil {
				return err
			}
			// NOTE: we don't want help output on errors from here onwards
			cmd.SilenceUsage = true
			return o.run(cmd.Context(), log, host)
		},
	}
	fs := cmd.Flags()
	fs.StringVar(&o.outputType, "output-type", string(generate.OutputBoth), "Files to generate: isc, disc or both")
	fs.StringVar(&o.outputDir, "output-dir", ".", "Directory the generated files are written to")
	fs.StringVar(&o.prefix, "prefix", "", "Only consider repositories starting with this registry path")
	fs.StringSliceVar(&o.repos, "repos", nil, "Repositories to inspect, for registries where /v2/_catalog is disabled")
	return cmd
}

func (o *generateOptions) validate(args []string) (string, error) {
	switch generate.OutputType(o.outputType) {
	case generate.OutputISC, generate.OutputDISC, generate.OutputBoth:
	default:
		return "", fmt.Errorf("invalid --output-type %q, must be isc, disc or both", o.outputType)
	}
	if len(args) < 1 {
		return "", errors.New("the registry argument is required, for example docker://registry.example.com:5000")
	}
	if !strings.HasPrefix(args[0], consts.DockerProtocol) {
		return "", fmt.Errorf("the registry argument must have a %s prefix", consts.DockerProtocol)
	}
	host := strings.TrimPrefix(args[0], consts.DockerProtocol)
	if strings.Contains(host, "/") {
		return "", errors.New("the registry argument must be host[:port] only, use --prefix to restrict to a registry path")
	}
	return host, nil
}

func (o *generateOptions) run(ctx context.Context, log clog.PluggableLoggerInterface, host string) error {
	sysCtx, err := o.globalOpts.SrcImage.NewSystemContext()
	if err != nil {
		return err
	}
	src := &generate.RegistrySource{
		Host:   host,
		Repos:  o.repos,
		SysCtx: sysCtx,
		LoadCatalog: func(ctx context.Context, ref string) (model.Model, error) {
			return list.DownloadCatalog(ctx, log, ref, *o.globalOpts)
		},
	}
	content, err := (&generate.Generator{Source: src, Host: host, Prefix: o.prefix}).Discover(ctx)
	if err != nil {
		return err
	}
	for _, w := range content.Warnings {
		log.Warn("%s", w)
	}
	paths, err := generate.WriteFiles(o.outputDir, content, generate.OutputType(o.outputType))
	if err != nil {
		return err
	}
	for _, p := range paths {
		log.Info("wrote %s", p)
	}
	return nil
}
```

`executor.go` after line 249: `cmd.AddCommand(NewGenerateCommand(log, opts))`.

`docs/features/generate-config.md`: usage, flags table, what is inferred (releases, operators, additional), what is not (Helm charts, original channel names, original source locations, digest-mirrored additional images), the `/v2/_catalog` and `--repos` note, never overwrites existing files, and the review-before-use warning.

- [ ] **Step 4: Run** `go test ... ./internal/pkg/cli/...` and `./internal/pkg/generate/...`, expect PASS.
- [ ] **Step 5: Commit** `feat(cli): add generate command`

---

### Task 7: Verify end to end and publish

- [ ] **Step 1:** `make build`, expect success. Then `./bin/oc-mirror --v2 generate --help` shows the flags.
- [ ] **Step 2:** Manual smoke test against a local registry populated by the test helper or a real `registry:2` instance (`podman run -d -p 5000:5000 registry:2`, push a tagged image, run `./bin/oc-mirror --v2 generate docker://localhost:5000 --src-tls-verify=false --output-dir <scratch dir>`). Confirm the output parses with `oc-mirror --v2 --config <file> ... --dry-run` or `config.ReadConfig`. Report honestly if this step is skipped (no container runtime).
- [ ] **Step 3:** `make sanity`, expect clean (golangci-lint via `make verify` as well). Fix findings in new code only.
- [ ] **Step 4:** `git status` and scan the diff for secrets (rule 15).
- [ ] **Step 5:** Push: `git push -u ammachado feature/generate-config-from-registry`.
