package image

import (
	"testing"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/stretchr/testify/require"
)

const (
	testKeyName    = "testKey"
	setTestKeyName = "setTestKey"
)

func TestUpdateKey(t *testing.T) {
	asSet := makeTestAssocationSet()
	testAssocs, ok := asSet[setTestKeyName]
	if !ok {
		t.Fatal()
	}
	require.NoError(t, asSet.UpdateKey(setTestKeyName, "update"))
	updateAssocs, ok := asSet["update"]
	if !ok {
		t.Fatal()
	}
	require.Equal(t, testAssocs, updateAssocs)
}

func TestUpdateValue(t *testing.T) {
	asSet := makeTestAssocationSet()
	newAssoc := v1alpha2.Association{
		Name:       testKeyName,
		Path:       "new",
		ID:         "new-id",
		TagSymlink: "new-tag",
		Type:       v1alpha2.TypeGeneric,
	}
	require.NoError(t, asSet.UpdateValue(setTestKeyName, newAssoc))
	assoc, ok := asSet[setTestKeyName][newAssoc.Name]
	if !ok {
		t.Fatal()
	}
	require.Equal(t, newAssoc, assoc)
}

func TestMerge(t *testing.T) {
	asSet := makeTestAssocationSet()
	newASSet := AssociationSet{}
	newAssocs := Associations{}
	newAssoc := v1alpha2.Association{
		Name:       "newKey",
		Path:       "new",
		ID:         "new-id",
		TagSymlink: "new-tag",
		Type:       v1alpha2.TypeGeneric,
	}
	newAssocs["newKey"] = newAssoc
	newASSet["setNewKey"] = newAssocs
	asSet.Merge(newASSet)
}

func TestContainsKey(t *testing.T) {
	asSet := makeTestAssocationSet()
	require.Equal(t, true, asSet.ContainsKey(setTestKeyName, testKeyName))
}
func TestSetContainsKey(t *testing.T) {
	asSet := makeTestAssocationSet()
	require.Equal(t, true, asSet.SetContainsKey(setTestKeyName))
}
func TestKeys(t *testing.T) {
	asSet := makeTestAssocationSet()
	require.Equal(t, []string{setTestKeyName}, asSet.Keys())
}

func TestSearch(t *testing.T) {
	asSet := makeTestAssocationSet()
	assocs, ok := asSet.Search(setTestKeyName)
	if !ok {
		t.Fatal()
	}
	require.Len(t, assocs, 1)
}

func TestGetDigests(t *testing.T) {
	asSet := makeTestAssocationSet()
	digests := asSet.GetDigests()
	require.Len(t, digests, 2)
}

func TestAdd(t *testing.T) {
	asSet := AssociationSet{}
	newAssoc := v1alpha2.Association{
		Name:       testKeyName,
		Path:       "new",
		ID:         "new-id",
		TagSymlink: "new-tag",
		Type:       v1alpha2.TypeGeneric,
	}
	asSet.Add(setTestKeyName, newAssoc)
	assocs, ok := asSet[setTestKeyName]
	if !ok {
		t.Fatal()
	}
	assoc, ok := assocs[testKeyName]
	if !ok {
		t.Fatal()
	}
	require.Equal(t, newAssoc, assoc)
}

func TestReposForBlobs(t *testing.T) {
	asSet := makeTestAssocationSet()
	ref := AssocPathsForBlobs(asSet)
	exp := map[string]string{
		"test-layer": "test",
	}
	require.Equal(t, exp, ref)
}

func makeTestAssocationSet() AssociationSet {
	asSet := AssociationSet{}
	assocs := Associations{}
	association := v1alpha2.Association{
		Name:         testKeyName,
		Path:         "test",
		ID:           "test-id",
		TagSymlink:   "test-tag",
		Type:         v1alpha2.TypeGeneric,
		LayerDigests: []string{"test-layer"},
	}
	assocs[testKeyName] = association
	asSet[setTestKeyName] = assocs
	return asSet
}
