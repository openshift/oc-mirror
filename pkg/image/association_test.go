package image

import (
	"testing"

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
	newAssoc := Association{
		Name:       testKeyName,
		Path:       "new",
		ID:         "new-id",
		TagSymlink: "new-tag",
		Type:       TypeGeneric,
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
	newAssoc := Association{
		Name:       "newKey",
		Path:       "new",
		ID:         "new-id",
		TagSymlink: "new-tag",
		Type:       TypeGeneric,
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
	newAssoc := Association{
		Name:       testKeyName,
		Path:       "new",
		ID:         "new-id",
		TagSymlink: "new-tag",
		Type:       TypeGeneric,
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

func TestGetImageFromBlob(t *testing.T) {
	asSet := makeTestAssocationSet()
	ref := GetImageFromBlob(asSet, "test-layer")
	require.Equal(t, setTestKeyName, ref)
	ref = GetImageFromBlob(asSet, "fake")
	require.Equal(t, "", ref)
}

func makeTestAssocationSet() AssociationSet {
	asSet := AssociationSet{}
	assocs := Associations{}
	association := Association{
		Name:         testKeyName,
		Path:         "test",
		ID:           "test-id",
		TagSymlink:   "test-tag",
		Type:         TypeGeneric,
		LayerDigests: []string{"test-layer"},
	}
	assocs[testKeyName] = association
	asSet[setTestKeyName] = assocs
	return asSet
}
