package list

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/operator-framework/operator-registry/alpha/model"
	"github.com/operator-framework/operator-registry/alpha/property"
	"github.com/stretchr/testify/assert"

	"github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func populateValidCatalogModel(t *testing.T) model.Model {
	t.Helper()

	m := make(model.Model)

	// Package with display name
	raw, err := json.Marshal(&property.CSVMetadata{DisplayName: "Red Hat Integration - 3scale"})
	assert.NoError(t, err)
	pkg := &model.Package{Name: "3scale-operator"}
	pkg.Channels = make(map[string]*model.Channel)
	pkg.Channels["threescale-2.16"] = &model.Channel{
		Name:    "threescale-2.16",
		Package: pkg,
		Bundles: map[string]*model.Bundle{
			"3scale-operator.v0.13.2": {
				Name:    "3scale-operator.v0.13.2",
				Package: pkg,
				Version: semver.MustParse("0.13.2"),
				Properties: []property.Property{
					{
						Type:  property.TypeCSVMetadata,
						Value: raw,
					},
				},
			},
		},
	}
	pkg.Channels["threescale-mas"] = &model.Channel{
		Name:    "threescale-mas",
		Package: pkg,
		Bundles: map[string]*model.Bundle{
			"3scale-operator.v0.11.8-mas": {
				Name:    "3scale-operator.v0.11.8-mas",
				Package: pkg,
			},
		},
	}
	pkg.DefaultChannel = pkg.Channels["threescale-2.16"]
	m["3scale-operator"] = pkg

	// Package without display name, and bundles
	pkg2 := &model.Package{
		Name:     "web-terminal",
		Channels: map[string]*model.Channel{"fast": {Name: "fast"}},
	}
	m["web-terminal"] = pkg2

	// Package without display name, channels, and bundles
	pkg3 := &model.Package{Name: "mtv-operator"}
	m["mtv-operator"] = pkg3

	return m
}

func TestListCatalogs(t *testing.T) {
	t.Skip("needs internet access")

	log := log.New("debug")
	opts := &mirror.CopyOptions{Global: &mirror.GlobalOptions{}}

	t.Run("should succeed", func(t *testing.T) {
		w := strings.Builder{}
		err := listCatalogsForVersion(t.Context(), log, &w, "4.20", *opts)
		assert.NoError(t, err)
		assert.Equal(t, `Available OpenShift OperatorHub catalogs:
OpenShift 4.20:
registry.redhat.io/redhat/redhat-operator-index:v4.20
registry.redhat.io/redhat/certified-operator-index:v4.20
registry.redhat.io/redhat/community-operator-index:v4.20
registry.redhat.io/redhat/redhat-marketplace-index:v4.20
`, w.String())
	})

	t.Run("should fail when OCP version is invalid", func(t *testing.T) {
		w := strings.Builder{}
		err := listCatalogsForVersion(t.Context(), log, &w, "foobar", *opts)
		assert.ErrorContains(t, err, "failed to check catalog")
	})
}

func TestListOperators(t *testing.T) {
	t.Run("should succeed when", func(t *testing.T) {
		t.Run("operator has no display name and no channels", func(t *testing.T) {
			w := strings.Builder{}
			m := model.Model{"3scale-operator": &model.Package{Name: "3scale-operator"}}
			err := listOperators(&w, m)
			assert.NoError(t, err)
			assert.Equal(t, "NAME\tDISPLAY NAME\tDEFAULT CHANNEL\n3scale-operator\t\t\n", w.String())
		})
		t.Run("operator has no display name and no default channel", func(t *testing.T) {
			w := strings.Builder{}
			m := model.Model{
				"3scale-operator": &model.Package{
					Name: "3scale-operator",
					Channels: map[string]*model.Channel{
						"threescale-mas": {
							Name: "threescale-mas",
							Bundles: map[string]*model.Bundle{
								"3scale-operator.v0.11.8-mas": {
									Name: "3scale-operator.v0.11.8-mas",
								},
							},
						},
					},
				},
			}
			err := listOperators(&w, m)
			assert.NoError(t, err)
			assert.Equal(t, "NAME\tDISPLAY NAME\tDEFAULT CHANNEL\n3scale-operator\t\t\n", w.String())
		})
		t.Run("operator has no display name and default channel", func(t *testing.T) {
			defCh := &model.Channel{
				Name: "threescale-2.16",
				Bundles: map[string]*model.Bundle{
					"3scale-operator.v0.13.2": {Name: "3scale-operator.v0.13.2"},
				},
			}
			m := model.Model{
				"3scale-operator": &model.Package{
					Name:           "3scale-operator",
					DefaultChannel: defCh,
					Channels:       map[string]*model.Channel{"threescale-2.16": defCh},
				},
			}
			w := strings.Builder{}
			err := listOperators(&w, m)
			assert.NoError(t, err)
			assert.Equal(t, "NAME\tDISPLAY NAME\tDEFAULT CHANNEL\n3scale-operator\t\tthreescale-2.16\n", w.String())
		})
		t.Run("operator has no default channel", func(t *testing.T) {
			m := model.Model{
				"3scale-operator": &model.Package{
					Name:     "3scale-operator",
					Channels: map[string]*model.Channel{"threescale-2.16": {Name: "threescale-2.16"}},
				},
			}
			w := strings.Builder{}
			err := listOperators(&w, m)
			assert.NoError(t, err)
			assert.Equal(t, "NAME\tDISPLAY NAME\tDEFAULT CHANNEL\n3scale-operator\t\t\n", w.String())
		})
		t.Run("operator has display name and default channel", func(t *testing.T) {
			w := strings.Builder{}
			m := populateValidCatalogModel(t)
			err := listOperators(&w, m)
			assert.NoError(t, err)
			assert.Equal(t, `NAME	DISPLAY NAME	DEFAULT CHANNEL
3scale-operator	Red Hat Integration - 3scale	threescale-2.16
mtv-operator		
web-terminal		
`, w.String())
		})
	})
}

func TestListChannels(t *testing.T) {
	t.Run("should succeed when", func(t *testing.T) {
		t.Run("operator has no channels", func(t *testing.T) {
			w := strings.Builder{}
			m := populateValidCatalogModel(t)
			err := listChannels(&w, m["mtv-operator"])
			assert.NoError(t, err)
			assert.Equal(t, "PACKAGE\tCHANNEL\tHEAD\n", w.String())
		})
		t.Run("channel has no head", func(t *testing.T) {
			w := strings.Builder{}
			m := populateValidCatalogModel(t)
			err := listChannels(&w, m["web-terminal"])
			assert.NoError(t, err)
			assert.Equal(t, "PACKAGE\tCHANNEL\tHEAD\nweb-terminal\tfast\tERROR: no channel head found in graph\n", w.String())
		})
	})
}

func TestListBundles(t *testing.T) {
	m := populateValidCatalogModel(t)

	t.Run("should succeed when", func(t *testing.T) {
		t.Run("channel has bundles", func(t *testing.T) {
			w := strings.Builder{}
			err := listBundles(&w, m["3scale-operator"].Channels["threescale-2.16"])
			assert.NoError(t, err)
			assert.Equal(t, "VERSIONS\n0.13.2\n", w.String())
		})
		t.Run("channel has no bundles", func(t *testing.T) {
			w := strings.Builder{}
			err := listBundles(&w, m["web-terminal"].Channels["fast"])
			assert.NoError(t, err)
			assert.Equal(t, "VERSIONS\n", w.String())
		})
	})
}
