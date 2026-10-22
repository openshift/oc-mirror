package operator

import (
	"context"
	"testing"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/model"
	"github.com/operator-framework/operator-registry/alpha/property"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/generate"
)

// catalogSource serves one catalog model to the generator.
type catalogSource struct{ m model.Model }

func (catalogSource) Repositories(context.Context) ([]string, error) {
	return []string{"team/idx"}, nil
}
func (catalogSource) Tags(context.Context, string) ([]string, error) { return []string{"v4.16"}, nil }
func (catalogSource) IsCatalog(context.Context, string, string) (bool, error) {
	return true, nil
}
func (c catalogSource) CatalogModel(context.Context, string, string) (model.Model, error) {
	return c.m, nil
}

// TestGeneratedOperatorConfigIsAcceptedByCatalogFilter checks the operators
// produced by `generate` pass the same filter used when mirroring, and keep
// every bundle of the already-filtered catalog.
func TestGeneratedOperatorConfigIsAcceptedByCatalogFilter(t *testing.T) {
	pkgProp := func(v string) []property.Property {
		return []property.Property{property.MustBuildPackage("foo", v)}
	}
	dc := declcfg.DeclarativeConfig{
		Packages: []declcfg.Package{{Schema: declcfg.SchemaPackage, Name: "foo", DefaultChannel: "stable"}},
		Channels: []declcfg.Channel{{Schema: declcfg.SchemaChannel, Package: "foo", Name: "stable", Entries: []declcfg.ChannelEntry{
			{Name: "foo.v1.0.0"}, {Name: "foo.v1.1.0", Replaces: "foo.v1.0.0"},
		}}},
		Bundles: []declcfg.Bundle{
			{Schema: declcfg.SchemaBundle, Name: "foo.v1.0.0", Package: "foo", Image: "r/foo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Properties: pkgProp("1.0.0")},
			{Schema: declcfg.SchemaBundle, Name: "foo.v1.1.0", Package: "foo", Image: "r/foo@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Properties: pkgProp("1.1.0")},
		},
	}
	m, err := declcfg.ConvertToModel(dc)
	require.NoError(t, err)

	content, err := (&generate.Generator{Source: catalogSource{m}, Host: "h"}).Discover(context.Background())
	require.NoError(t, err)
	require.Len(t, content.Operators, 1)

	filtered, err := filterCatalog(context.Background(), dc, content.Operators[0])
	require.NoError(t, err)
	assert.Len(t, filtered.Bundles, 2)
}
