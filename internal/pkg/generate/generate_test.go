package generate

import (
	"context"
	"errors"
	"strings"
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
	tagsErr  error
	probeErr error
	modelErr error
	probed   []string // repo:tag values passed to IsCatalog
}

// Repositories returns fixture repositories or the configured enumeration error.
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

// Tags returns fixture tags or the configured tag error.
func (f *fakeSource) Tags(_ context.Context, repo string) ([]string, error) {
	if f.tagsErr != nil {
		return nil, f.tagsErr
	}
	return f.tags[repo], nil
}

// IsCatalog records probes and checks the catalog fixtures.
func (f *fakeSource) IsCatalog(_ context.Context, repo, tag string) (bool, error) {
	f.probed = append(f.probed, repo+":"+tag)
	if f.probeErr != nil {
		return false, f.probeErr
	}
	_, ok := f.catalogs[repo+":"+tag]
	return ok, nil
}

// CatalogModel returns fixture catalog content or the configured loading error.
func (f *fakeSource) CatalogModel(_ context.Context, repo, tag string) (model.Model, error) {
	if f.modelErr != nil {
		return nil, f.modelErr
	}
	return f.catalogs[repo+":"+tag], nil
}

// TestDiscover checks classification across release, operator, and additional repositories.
func TestDiscover(t *testing.T) {
	pkg := &model.Package{Name: "foo"}
	pkg.Channels = map[string]*model.Channel{"stable": {Name: "stable", Package: pkg,
		Bundles: map[string]*model.Bundle{"foo.v1": {Name: "foo.v1", Version: semver.MustParse("1.0.0")}}}}
	src := &fakeSource{
		tags: map[string][]string{
			"openshift/release-images":     {"4.16.3-x86_64"},
			"openshift/release":            {"4.16.3-x86_64-etcd"},
			"redhat/redhat-operator-index": {"v4.16"},
			"ubi9/ubi":                     {"latest", "0123456789ab"},
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
	assert.Contains(t, c.Warnings, "1 digest-derived tags skipped (operator related images, release components, Helm images); they cannot be told apart from digest-mirrored additional images")
	assertWarningContaining(t, c.Warnings, "platform channels are inferred")
	assertWarningContaining(t, c.Warnings, "point at the mirror registry")
}

// TestDiscoverPrefix checks discovery is restricted to the requested repository path.
func TestDiscoverPrefix(t *testing.T) {
	src := &fakeSource{tags: map[string][]string{"team-a/app": {"1"}, "team-b/app": {"1"}}}
	c, err := (&Generator{Source: src, Host: "h", Prefix: "team-a/"}).Discover(context.Background())
	require.NoError(t, err)
	require.Len(t, c.AdditionalImages, 1)
	assert.Equal(t, "h/team-a/app:1", c.AdditionalImages[0].Name)
}

// TestDiscoverRepoError checks repository enumeration failures propagate.
func TestDiscoverRepoError(t *testing.T) {
	src := &fakeSource{repoErr: errors.New("boom")}
	_, err := (&Generator{Source: src, Host: "h"}).Discover(context.Background())
	assert.ErrorContains(t, err, "boom")
}

// assertWarningContaining asserts at least one warning contains the expected explanation.
func assertWarningContaining(t *testing.T, warnings []string, substr string) {
	t.Helper()
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return
		}
	}
	t.Errorf("no warning contains %q; got %v", substr, warnings)
}

// TestDiscoverTagsError checks tag enumeration failures retain their context.
func TestDiscoverTagsError(t *testing.T) {
	src := &fakeSource{tags: map[string][]string{"team/app": {"1"}}, tagsErr: errors.New("tags boom")}
	_, err := (&Generator{Source: src, Host: "h"}).Discover(context.Background())
	assert.ErrorContains(t, err, `listing tags for "team/app"`)
	assert.ErrorContains(t, err, "tags boom")
}

// A failing per-repo probe no longer aborts the run (behavior changed deliberately):
// the repo becomes an additional image and a warning names the cause.
func TestDiscoverIsCatalogError(t *testing.T) {
	src := &fakeSource{tags: map[string][]string{"team/app": {"1"}}, probeErr: errors.New("probe boom")}
	c, err := (&Generator{Source: src, Host: "h"}).Discover(context.Background())
	require.NoError(t, err)
	require.Len(t, c.AdditionalImages, 1)
	assert.Equal(t, "h/team/app:1", c.AdditionalImages[0].Name)
	assertWarningContaining(t, c.Warnings, "team/app:1")
	assertWarningContaining(t, c.Warnings, "probe boom")
}

// TestDiscoverCatalogModelError checks catalog loading failures propagate with image context.
func TestDiscoverCatalogModelError(t *testing.T) {
	src := &fakeSource{
		tags:     map[string][]string{"team/index": {"v1"}},
		catalogs: map[string]model.Model{"team/index:v1": {}},
		modelErr: errors.New("model boom"),
	}
	_, err := (&Generator{Source: src, Host: "h"}).Discover(context.Background())
	assert.ErrorContains(t, err, "reading catalog team/index:v1")
	assert.ErrorContains(t, err, "model boom")
}

// TestDiscoverSignatureTags checks signatures are excluded from catalog probes.
func TestDiscoverSignatureTags(t *testing.T) {
	src := &fakeSource{tags: map[string][]string{
		"team/app": {"sha256-" + strings.Repeat("a", 64) + ".sig", "v1"},
	}}
	c, err := (&Generator{Source: src, Host: "h"}).Discover(context.Background())
	require.NoError(t, err)
	require.Len(t, c.AdditionalImages, 1)
	assert.Equal(t, "h/team/app:v1", c.AdditionalImages[0].Name)
	assert.Equal(t, []string{"team/app:v1"}, src.probed, "catalog probe must use a non-signature tag")
}

// TestDiscoverPrefixTargets checks generated targets omit the destination prefix.
func TestDiscoverPrefixTargets(t *testing.T) {
	pkg := &model.Package{Name: "foo"}
	pkg.Channels = map[string]*model.Channel{}
	src := &fakeSource{
		tags: map[string][]string{
			"mirror/ubi9/ubi":                     {"latest"},
			"mirror/redhat/redhat-operator-index": {"v4.16"},
		},
		catalogs: map[string]model.Model{"mirror/redhat/redhat-operator-index:v4.16": {"foo": pkg}},
	}
	c, err := (&Generator{Source: src, Host: "h", Prefix: "mirror"}).Discover(context.Background())
	require.NoError(t, err)
	require.Len(t, c.AdditionalImages, 1)
	assert.Equal(t, "h/mirror/ubi9/ubi:latest", c.AdditionalImages[0].Name)
	assert.Equal(t, "ubi9/ubi", c.AdditionalImages[0].TargetRepo)
	require.Len(t, c.Operators, 1)
	assert.Equal(t, "redhat/redhat-operator-index", c.Operators[0].TargetCatalog)
}

// TestDiscoverNoPrefixNoTargets checks unprefixed discovery leaves target overrides unset.
func TestDiscoverNoPrefixNoTargets(t *testing.T) {
	src := &fakeSource{tags: map[string][]string{"ubi9/ubi": {"latest"}}}
	c, err := (&Generator{Source: src, Host: "h"}).Discover(context.Background())
	require.NoError(t, err)
	assert.Empty(t, c.AdditionalImages[0].TargetRepo)
}

// TestDiscoverPrefixBoundary checks a prefix does not select sibling path names.
func TestDiscoverPrefixBoundary(t *testing.T) {
	src := &fakeSource{tags: map[string][]string{"team-a/app": {"1"}, "team-ab/app": {"1"}}}
	for _, p := range []string{"team-a", "team-a/"} {
		c, err := (&Generator{Source: src, Host: "h", Prefix: p}).Discover(context.Background())
		require.NoError(t, err)
		require.Len(t, c.AdditionalImages, 1, p)
		assert.Equal(t, "h/team-a/app:1", c.AdditionalImages[0].Name)
	}
}

type countingSource struct {
	*fakeSource
	tagCalls []string
}

// Tags records tag requests before delegating to the fixture source.
func (c *countingSource) Tags(ctx context.Context, repo string) ([]string, error) {
	c.tagCalls = append(c.tagCalls, repo)
	return c.fakeSource.Tags(ctx, repo)
}

// TestDiscoverDoesNotListReleaseContentTags checks release component repositories are skipped before tag enumeration.
func TestDiscoverDoesNotListReleaseContentTags(t *testing.T) {
	src := &countingSource{fakeSource: &fakeSource{tags: map[string][]string{
		"openshift/release": {"x"}, "openshift/release-images": {"4.16.3-x86_64"},
	}}}
	_, err := (&Generator{Source: src, Host: "h"}).Discover(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"openshift/release-images"}, src.tagCalls)
}

// TestDiscoverSkipsExactPrefix protects root destinations from prefix duplication.
func TestDiscoverSkipsExactPrefix(t *testing.T) {
	for _, prefix := range []string{"mirror", "mirror/"} {
		for _, catalog := range []bool{false, true} {
			src := &countingSource{fakeSource: &fakeSource{tags: map[string][]string{"mirror": {"v1"}, "mirror/app": {"v1"}}}}
			if catalog {
				src.catalogs = map[string]model.Model{"mirror:v1": {}}
			}
			c, err := (&Generator{Source: src, Host: "h", Prefix: prefix}).Discover(context.Background())
			require.NoError(t, err)
			require.Len(t, c.AdditionalImages, 1)
			assert.Equal(t, "app", c.AdditionalImages[0].TargetRepo)
			assert.Empty(t, c.Operators)
			assert.Equal(t, []string{"mirror/app"}, src.tagCalls)
			assertWarningContaining(t, c.Warnings, "equals the prefix")
		}
	}
}
