package delete

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

// These tests cover delete plans in which several selected images share a destination reference
// (same repository and tag) but address different content. A registry tag can only point at one
// manifest, so the plan must address such content by digest.
//
// The entry builders below reproduce what the additional images collector emits in delete
// mode (see additional.LocalStorageCollector.buildDiskToMirrorPaths).

// colTagDigestEntry: repo:tag@digest -> cache keyed by digest, destination keyed by the human tag.
func colTagDigestEntry(repo, tag, dgst string) v2alpha1.CopyImageSchema {
	return v2alpha1.CopyImageSchema{
		Source:      "docker://" + colCacheFQDN + "/" + repo + ":" + colDigestTag(dgst),
		Destination: colDeleteDestination + "/" + repo + ":" + tag,
		Origin:      "test.registry.io/" + repo + ":" + tag + "@" + dgst,
		Type:        v2alpha1.TypeGeneric,
	}
}

// colDigestTargetTagEntry: repo@digest with targetTag -> cache and destination keyed by the target tag.
func colDigestTargetTagEntry(repo, targetTag, dgst string) v2alpha1.CopyImageSchema {
	return v2alpha1.CopyImageSchema{
		Source:      "docker://" + colCacheFQDN + "/" + repo + ":" + targetTag,
		Destination: colDeleteDestination + "/" + repo + ":" + targetTag,
		Origin:      "test.registry.io/" + repo + "@" + dgst,
		Type:        v2alpha1.TypeGeneric,
	}
}

// colV1Entry: repo@digest with --delete-v1-images -> cache keyed by digest, destination ":latest".
func colV1Entry(repo, dgst string) v2alpha1.CopyImageSchema {
	return v2alpha1.CopyImageSchema{
		Source:      "docker://" + colCacheFQDN + "/" + repo + ":" + colDigestTag(dgst),
		Destination: colDeleteDestination + "/" + repo + ":latest",
		Origin:      "test.registry.io/" + repo + "@" + dgst,
		Type:        v2alpha1.TypeGeneric,
	}
}

// colTagOnlyEntry: repo:tag -> cache and destination keyed by the tag.
func colTagOnlyEntry(originRepo, destRepo, tag string) v2alpha1.CopyImageSchema {
	return v2alpha1.CopyImageSchema{
		Source:      "docker://" + colCacheFQDN + "/" + destRepo + ":" + tag,
		Destination: colDeleteDestination + "/" + destRepo + ":" + tag,
		Origin:      "test.registry.io/" + originRepo + ":" + tag,
		Type:        v2alpha1.TypeGeneric,
	}
}

// generateCollisionPlan runs the real generation consumer and reads the written plan back.
func generateCollisionPlan(t *testing.T, di DeleteImages, images []v2alpha1.CopyImageSchema) []v2alpha1.DeleteItem {
	t.Helper()
	require.NoError(t, di.WriteDeleteMetaData(context.Background(), images))
	list, err := di.ReadDeleteMetaData()
	require.NoError(t, err)
	return list.Items
}

func colReferences(items []v2alpha1.DeleteItem) []string {
	refs := make([]string, 0, len(items))
	for _, item := range items {
		refs = append(refs, item.ImageReference)
	}
	sort.Strings(refs)
	return refs
}

// digestSigHandler stands in for the cosign tag lookup: it reports the signature tag of
// the digest the cache reference resolves to. A digest-keyed cache tag (sha256-<hex>) or a
// digest reference resolves to that digest; any other (shared) tag resolves to sharedDigest,
// like a cache in which only one manifest can hold the tag.
type digestSigHandler struct {
	sharedDigest string
	fail         bool
}

func (h *digestSigHandler) GetSignatureTag(_ context.Context, imgRef string) ([]string, error) {
	if h.fail {
		return nil, errors.New("signature lookup unavailable")
	}
	if idx := strings.LastIndex(imgRef, "@"); idx >= 0 {
		return []string{colDigestTag(imgRef[idx+1:]) + ".sig"}, nil
	}
	tag := imgRef[strings.LastIndex(imgRef, ":")+1:]
	if strings.HasPrefix(tag, "sha256-") {
		return []string{tag + ".sig"}, nil
	}
	return []string{colDigestTag(h.sharedDigest) + ".sig"}, nil
}

func TestDeletePlanKeepsDistinctDigestsSharingDestinationTag(t *testing.T) {
	tests := []struct {
		name     string
		images   []v2alpha1.CopyImageSchema
		expected map[string]string // imageReference -> imageName
	}{
		{
			name:   "tag and digest images sharing a tag",
			images: []v2alpha1.CopyImageSchema{colTagDigestEntry(colRepo, "v1", colDigestA), colTagDigestEntry(colRepo, "v1", colDigestB)},
			expected: map[string]string{
				colDestRepo + "@" + colDigestA: "test.registry.io/" + colRepo + ":v1@" + colDigestA,
				colDestRepo + "@" + colDigestB: "test.registry.io/" + colRepo + ":v1@" + colDigestB,
			},
		},
		{
			name:   "digest only images sharing a targetTag",
			images: []v2alpha1.CopyImageSchema{colDigestTargetTagEntry("testns/test", "v1.0", colDigestA), colDigestTargetTagEntry("testns/test", "v1.0", colDigestB)},
			expected: map[string]string{
				colDeleteDestination + "/testns/test@" + colDigestA: "test.registry.io/testns/test@" + colDigestA,
				colDeleteDestination + "/testns/test@" + colDigestB: "test.registry.io/testns/test@" + colDigestB,
			},
		},
		{
			name:   "delete-v1-images digest only images sharing latest",
			images: []v2alpha1.CopyImageSchema{colV1Entry("testns/v1img", colDigestA), colV1Entry("testns/v1img", colDigestB)},
			expected: map[string]string{
				colDeleteDestination + "/testns/v1img@" + colDigestA: "test.registry.io/testns/v1img@" + colDigestA,
				colDeleteDestination + "/testns/v1img@" + colDigestB: "test.registry.io/testns/v1img@" + colDigestB,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			di := newCollisionDeleteImages(newCollisionOpts(t, false, false), &recordingBatch{}, &resolvingManifest{}, &digestSigHandler{})
			items := generateCollisionPlan(t, di, tt.images)
			require.Len(t, items, len(tt.expected))
			for _, item := range items {
				name, ok := tt.expected[item.ImageReference]
				require.True(t, ok, "unexpected imageReference %s", item.ImageReference)
				assert.Equal(t, name, item.ImageName)
				assert.Equal(t, v2alpha1.TypeGeneric, item.Type)
			}
		})
	}
}

func TestDeletePlanDeduplicatesExactRepeats(t *testing.T) {
	t.Run("repeated entry inside a collision group", func(t *testing.T) {
		di := newCollisionDeleteImages(newCollisionOpts(t, false, true), &recordingBatch{}, &resolvingManifest{}, &digestSigHandler{})
		a := colTagDigestEntry(colRepo, "v1", colDigestA)
		items := generateCollisionPlan(t, di, []v2alpha1.CopyImageSchema{a, a, colTagDigestEntry(colRepo, "v1", colDigestB)})
		assert.Equal(t, colSorted(
			colDestRepo+"@"+colDigestA,
			colDestRepo+"@"+colDigestB,
			colDestRepo+":"+colDigestTag(colDigestA)+".sig",
			colDestRepo+":"+colDigestTag(colDigestB)+".sig",
		), colReferences(items))
	})

	t.Run("repeated entry without collision keeps the legacy reference", func(t *testing.T) {
		di := newCollisionDeleteImages(newCollisionOpts(t, false, false), &recordingBatch{}, &resolvingManifest{}, &digestSigHandler{})
		a := colTagDigestEntry(colRepo, "v1", colDigestA)
		items := generateCollisionPlan(t, di, []v2alpha1.CopyImageSchema{a, a})
		assert.Equal(t, []string{colDestRepo + ":v1"}, colReferences(items))
	})
}

func TestDeletePlanKeepsSameDigestInDifferentRepositories(t *testing.T) {
	di := newCollisionDeleteImages(newCollisionOpts(t, false, false), &recordingBatch{}, &resolvingManifest{}, &digestSigHandler{})
	items := generateCollisionPlan(t, di, []v2alpha1.CopyImageSchema{
		colTagDigestEntry("testns/repo-a", "v1", colDigestA),
		colTagDigestEntry("testns/repo-b", "v1", colDigestA),
	})
	assert.Equal(t, colSorted(
		colDeleteDestination+"/testns/repo-a:v1",
		colDeleteDestination+"/testns/repo-b:v1",
	), colReferences(items))
}

func TestDeletePlanSignaturesPerDigest(t *testing.T) {
	sigRefs := func(items []v2alpha1.DeleteItem) []string {
		var refs []string
		for _, ref := range colReferences(items) {
			if strings.HasSuffix(ref, ".sig") {
				refs = append(refs, ref)
			}
		}
		return refs
	}

	t.Run("tag and digest collision uses each digest keyed cache entry", func(t *testing.T) {
		di := newCollisionDeleteImages(newCollisionOpts(t, false, true), &recordingBatch{}, &resolvingManifest{}, &digestSigHandler{sharedDigest: colDigestC})
		items := generateCollisionPlan(t, di, []v2alpha1.CopyImageSchema{colTagDigestEntry(colRepo, "v1", colDigestA), colTagDigestEntry(colRepo, "v1", colDigestB)})
		assert.Equal(t, colSorted(
			colDestRepo+":"+colDigestTag(colDigestA)+".sig",
			colDestRepo+":"+colDigestTag(colDigestB)+".sig",
		), sigRefs(items))
	})

	t.Run("digest only targetTag collision does not reuse the shared cache tag", func(t *testing.T) {
		di := newCollisionDeleteImages(newCollisionOpts(t, false, true), &recordingBatch{}, &resolvingManifest{}, &digestSigHandler{sharedDigest: colDigestC})
		items := generateCollisionPlan(t, di, []v2alpha1.CopyImageSchema{colDigestTargetTagEntry("testns/test", "v1.0", colDigestA), colDigestTargetTagEntry("testns/test", "v1.0", colDigestB)})
		assert.Equal(t, colSorted(
			colDeleteDestination+"/testns/test:"+colDigestTag(colDigestA)+".sig",
			colDeleteDestination+"/testns/test:"+colDigestTag(colDigestB)+".sig",
		), sigRefs(items))
	})

	t.Run("lookup failure falls back to the origin digest", func(t *testing.T) {
		di := newCollisionDeleteImages(newCollisionOpts(t, false, true), &recordingBatch{}, &resolvingManifest{}, &digestSigHandler{fail: true})
		items := generateCollisionPlan(t, di, []v2alpha1.CopyImageSchema{colTagDigestEntry(colRepo, "v1", colDigestA), colTagDigestEntry(colRepo, "v1", colDigestB)})
		assert.Equal(t, colSorted(
			colDestRepo+":"+colDigestTag(colDigestA)+".sig",
			colDestRepo+":"+colDigestTag(colDigestB)+".sig",
		), sigRefs(items))
	})

	t.Run("operator catalogs never get signature items", func(t *testing.T) {
		di := newCollisionDeleteImages(newCollisionOpts(t, false, true), &recordingBatch{}, &resolvingManifest{}, &digestSigHandler{})
		catalog := colTagDigestEntry("redhat/redhat-operator-index", "v4.18", colDigestA)
		catalog.Type = v2alpha1.TypeOperatorCatalog
		items := generateCollisionPlan(t, di, []v2alpha1.CopyImageSchema{catalog})
		assert.Empty(t, sigRefs(items))
	})
}

func TestDeletePlanUnchangedWithoutCollision(t *testing.T) {
	catalogA := colTagDigestEntry("redhat/redhat-operator-index", "v4.18", colDigestA)
	catalogA.Type = v2alpha1.TypeOperatorCatalog
	catalogB := colTagDigestEntry("redhat/redhat-operator-index", "v4.18", colDigestB)
	catalogB.Type = v2alpha1.TypeOperatorCatalog

	tests := []struct {
		name     string
		images   []v2alpha1.CopyImageSchema
		expected []string
	}{
		{
			name:     "single tag only image",
			images:   []v2alpha1.CopyImageSchema{colTagOnlyEntry(colRepo, colRepo, "v1")},
			expected: []string{colDestRepo + ":v1"},
		},
		{
			name:     "single tag and digest image keeps the human tag",
			images:   []v2alpha1.CopyImageSchema{colTagDigestEntry(colRepo, "v1", colDigestA)},
			expected: []string{colDestRepo + ":v1"},
		},
		{
			name:     "operator catalogs sharing a destination are not pinned",
			images:   []v2alpha1.CopyImageSchema{catalogA, catalogB},
			expected: []string{colDeleteDestination + "/redhat/redhat-operator-index:v4.18"},
		},
		{
			name:     "tag only images remapped onto the same destination",
			images:   []v2alpha1.CopyImageSchema{colTagOnlyEntry("ns-a/app", colRepo, "v1"), colTagOnlyEntry("ns-b/app", colRepo, "v1")},
			expected: []string{colDestRepo + ":v1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			di := newCollisionDeleteImages(newCollisionOpts(t, false, false), &recordingBatch{}, &resolvingManifest{}, &digestSigHandler{})
			assert.Equal(t, tt.expected, colReferences(generateCollisionPlan(t, di, tt.images)))
		})
	}
}

func TestDeletePlanRejectsMixedTagAndDigestSelections(t *testing.T) {
	relatedA := colTagDigestEntry(colRepo, "v1", colDigestA)
	relatedA.Type = v2alpha1.TypeOperatorRelatedImage
	relatedA.Origin = "docker://" + relatedA.Origin

	tests := []struct {
		name        string
		images      []v2alpha1.CopyImageSchema
		destination string
		origins     []string
	}{
		{
			name:        "one known digest and a tag only image",
			images:      []v2alpha1.CopyImageSchema{colTagDigestEntry(colRepo, "v1", colDigestA), colTagOnlyEntry(colRepo, colRepo, "v1")},
			destination: colDestRepo + ":v1",
			origins:     []string{"test.registry.io/" + colRepo + ":v1@" + colDigestA, "test.registry.io/" + colRepo + ":v1"},
		},
		{
			name: "two known digests and a tag only image",
			images: []v2alpha1.CopyImageSchema{
				colTagDigestEntry(colRepo, "v1", colDigestA),
				colTagDigestEntry(colRepo, "v1", colDigestB),
				colTagOnlyEntry(colRepo, colRepo, "v1"),
			},
			destination: colDestRepo + ":v1",
			origins: []string{
				"test.registry.io/" + colRepo + ":v1@" + colDigestA,
				"test.registry.io/" + colRepo + ":v1@" + colDigestB,
				"test.registry.io/" + colRepo + ":v1",
			},
		},
		{
			name:        "digest only and tag only images sharing a targetTag",
			images:      []v2alpha1.CopyImageSchema{colDigestTargetTagEntry("testns/test", "v1.0", colDigestA), colTagOnlyEntry("other/app", "testns/test", "v1.0")},
			destination: colDeleteDestination + "/testns/test:v1.0",
			origins:     []string{"test.registry.io/testns/test@" + colDigestA, "test.registry.io/other/app:v1.0"},
		},
		{
			name:        "images from different collectors",
			images:      []v2alpha1.CopyImageSchema{relatedA, colTagOnlyEntry(colRepo, colRepo, "v1")},
			destination: colDestRepo + ":v1",
			origins:     []string{relatedA.Origin, "test.registry.io/" + colRepo + ":v1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := newCollisionOpts(t, false, false)
			di := newCollisionDeleteImages(opts, &recordingBatch{}, &resolvingManifest{}, &digestSigHandler{})

			err := di.WriteDeleteMetaData(context.Background(), tt.images)
			require.Error(t, err)
			assert.ErrorContains(t, err, "delete plan rejected")
			assert.ErrorContains(t, err, tt.destination)
			for _, origin := range tt.origins {
				assert.ErrorContains(t, err, origin)
			}

			for _, file := range []string{deleteImagesYaml, discYaml} {
				_, statErr := os.Stat(filepath.Join(opts.Global.WorkingDir, file))
				assert.True(t, os.IsNotExist(statErr), "%s must not be written when the plan is rejected", file)
			}
		})
	}

	t.Run("a previous plan is not overwritten", func(t *testing.T) {
		opts := newCollisionOpts(t, false, false)
		di := newCollisionDeleteImages(opts, &recordingBatch{}, &resolvingManifest{}, &digestSigHandler{})
		require.NoError(t, os.MkdirAll(filepath.Join(opts.Global.WorkingDir, "delete"), 0o755))
		for _, file := range []string{deleteImagesYaml, discYaml} {
			require.NoError(t, os.WriteFile(filepath.Join(opts.Global.WorkingDir, file), []byte("previous plan"), 0o600))
		}

		err := di.WriteDeleteMetaData(context.Background(), []v2alpha1.CopyImageSchema{colTagDigestEntry(colRepo, "v1", colDigestA), colTagOnlyEntry(colRepo, colRepo, "v1")})
		require.ErrorContains(t, err, "delete plan rejected")

		for _, file := range []string{deleteImagesYaml, discYaml} {
			data, readErr := os.ReadFile(filepath.Join(opts.Global.WorkingDir, file))
			require.NoError(t, readErr)
			assert.Equal(t, "previous plan", string(data))
		}
	})
}

func TestDeleteRegistryImagesDeletesEachCollidingDigest(t *testing.T) {
	images := []v2alpha1.CopyImageSchema{colTagDigestEntry(colRepo, "v1", colDigestA), colTagDigestEntry(colRepo, "v1", colDigestB)}

	t.Run("destination only", func(t *testing.T) {
		m := &resolvingManifest{}
		b := &recordingBatch{}
		di := newCollisionDeleteImages(newCollisionOpts(t, false, false), b, m, &digestSigHandler{})
		plan := generateCollisionPlan(t, di, images)

		require.NoError(t, di.DeleteRegistryImages(v2alpha1.DeleteImageList{Items: plan}))
		assert.Equal(t, 1, b.calls)
		assert.Equal(t, colSorted(colDestRepo+"@"+colDigestA, colDestRepo+"@"+colDigestB), b.destinations())
		assert.NotContains(t, b.destinations(), colDestRepo+":v1")
		assert.Equal(t, 2, b.schema.TotalAdditionalImages)
		assert.Empty(t, m.lookedUp(), "digest references need no lookup")
	})

	t.Run("with force-cache-delete", func(t *testing.T) {
		m := &resolvingManifest{manifests: map[string][]byte{
			colCacheRepo + ":" + colDigestTag(colDigestA): colManA,
			colCacheRepo + ":" + colDigestTag(colDigestB): colManB,
		}}
		b := &recordingBatch{}
		di := newCollisionDeleteImages(newCollisionOpts(t, true, false), b, m, &digestSigHandler{})
		plan := generateCollisionPlan(t, di, images)

		require.NoError(t, di.DeleteRegistryImages(v2alpha1.DeleteImageList{Items: plan}))
		assert.Equal(t, colSorted(
			colDestRepo+"@"+colDigestA,
			colDestRepo+"@"+colDigestB,
			colCacheRepo+"@"+colDigestA,
			colCacheRepo+"@"+colDigestB,
		), b.destinations())
		assert.Equal(t, 4, b.schema.TotalAdditionalImages)
		assert.Equal(t, colSorted(
			colCacheRepo+":"+colDigestTag(colDigestA),
			colCacheRepo+":"+colDigestTag(colDigestB),
		), m.lookedUp())
	})
}
