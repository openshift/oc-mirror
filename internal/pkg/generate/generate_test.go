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

func (f *fakeSource) Tags(_ context.Context, repo string) ([]string, error) {
	return f.tags[repo], nil
}

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
