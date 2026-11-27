package delete

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/opencontainers/go-digest"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/batch"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/openshift/oc-mirror/v2/internal/pkg/signature"
)

// These tests cover the deletion of plans addressing images of a repository by digest. In such a
// repository a tag and a digest (or two tags) can address the same manifest, and the deletion must
// remove every selected manifest exactly once, using the digest observed before anything is deleted.

const (
	colDeleteDestination = "docker://localhost:5000/myregistry"
	colCacheFQDN         = "localhost:8888"
	colRepo              = "testns/repo"
	colDestRepo          = colDeleteDestination + "/" + colRepo
	colCacheRepo         = "docker://" + colCacheFQDN + "/" + colRepo
)

// colManifest returns a valid OCI image manifest whose digest is unique per label.
func colManifest(label string) []byte {
	return []byte(fmt.Sprintf(`{"schemaVersion":2,"mediaType":"%s","config":{"mediaType":"%s","digest":"sha256:44d75007b39e0e1bbf1bcfd0721245add54c54c3f83903f8926fb4bef6827aa2","size":2},"layers":[],"annotations":{"test":"%s"}}`,
		specv1.MediaTypeImageManifest, specv1.MediaTypeImageConfig, label))
}

var (
	colManA    = colManifest("A")
	colManB    = colManifest("B")
	colManC    = colManifest("C")
	colManSigA = colManifest("signature-A")
	colManSigB = colManifest("signature-B")

	colDigestA    = digest.FromBytes(colManA).String()
	colDigestB    = digest.FromBytes(colManB).String()
	colDigestC    = digest.FromBytes(colManC).String()
	colDigestSigA = digest.FromBytes(colManSigA).String()
	colDigestSigB = digest.FromBytes(colManSigB).String()
)

// colDigestTag converts "sha256:<hex>" into the tag-safe "sha256-<hex>" form.
func colDigestTag(d string) string {
	return strings.Replace(d, ":", "-", 1)
}

// The entry builders below reproduce what the additional images collector emits in delete
// mode (see additional.LocalStorageCollector.buildDiskToMirrorPaths).

func newCollisionOpts(t *testing.T, forceCache, deleteSignatures bool) mirror.CopyOptions {
	t.Helper()
	global := &mirror.GlobalOptions{
		WorkingDir:        t.TempDir(),
		DeleteDestination: colDeleteDestination,
		ForceCacheDelete:  forceCache,
		DeleteSignatures:  deleteSignatures,
	}
	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()
	return mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Destination:         colDeleteDestination,
		Mode:                mirror.DiskToMirror,
		Function:            string(mirror.DeleteMode),
		LocalStorageFQDN:    colCacheFQDN,
	}
}

func newCollisionDeleteImages(opts mirror.CopyOptions, b batch.BatchInterface, m manifest.ManifestInterface, sig signature.SignatureInterface) DeleteImages {
	return DeleteImages{
		Log:              clog.New("trace"),
		Opts:             opts,
		Batch:            b,
		Blobs:            &mockBlobs{},
		Manifest:         m,
		SigHandler:       sig,
		LocalStorageDisk: "/tmp",
		LocalStorageFQDN: opts.LocalStorageFQDN,
	}
}

func colSorted(refs ...string) []string {
	out := append([]string{}, refs...)
	sort.Strings(out)
	return out
}

type colLookup struct {
	ref      string
	insecure types.OptionalBool
}

// resolvingManifest serves manifests for exact references and records every lookup with
// the TLS setting of the system context it was given.
type resolvingManifest struct {
	mockManifest
	mu        sync.Mutex
	manifests map[string][]byte
	errs      map[string]error
	lookups   []colLookup
}

func (m *resolvingManifest) ImageManifest(_ context.Context, sysCtx *types.SystemContext, imgRef string, _ *digest.Digest) ([]byte, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	call := colLookup{ref: imgRef}
	if sysCtx != nil {
		call.insecure = sysCtx.DockerInsecureSkipTLSVerify
	}
	m.lookups = append(m.lookups, call)
	if err, ok := m.errs[imgRef]; ok {
		return nil, "", err
	}
	if b, ok := m.manifests[imgRef]; ok {
		return b, specv1.MediaTypeImageManifest, nil
	}
	return nil, "", fmt.Errorf("manifest unknown: %s", imgRef)
}

func (m *resolvingManifest) lookedUp() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	refs := make([]string, 0, len(m.lookups))
	for _, l := range m.lookups {
		refs = append(refs, l.ref)
	}
	sort.Strings(refs)
	return refs
}

// recordingBatch records every Worker invocation; beforeReturn lets a test change registry
// state after the schema was handed over (e.g. retag after the preflight).
type recordingBatch struct {
	calls        int
	schema       v2alpha1.CollectorSchema
	beforeReturn func()
}

func (b *recordingBatch) Worker(_ context.Context, cs v2alpha1.CollectorSchema, _ mirror.CopyOptions) (v2alpha1.CollectorSchema, error) {
	b.calls++
	b.schema = cs
	if b.beforeReturn != nil {
		b.beforeReturn()
	}
	return cs, nil
}

func (b *recordingBatch) destinations() []string {
	refs := make([]string, 0, len(b.schema.AllImages))
	for _, img := range b.schema.AllImages {
		refs = append(refs, img.Destination)
	}
	sort.Strings(refs)
	return refs
}

func TestDeleteRegistryImagesPreflightAffectedRepositories(t *testing.T) {
	item := func(name, ref string) v2alpha1.DeleteItem {
		return v2alpha1.DeleteItem{ImageName: name, ImageReference: ref, Type: v2alpha1.TypeGeneric}
	}
	originV1A := "test.registry.io/" + colRepo + ":v1@" + colDigestA
	originV1B := "test.registry.io/" + colRepo + ":v1@" + colDigestB
	pinnedA := item(originV1A, colDestRepo+"@"+colDigestA)
	pinnedB := item(originV1B, colDestRepo+"@"+colDigestB)

	tests := []struct {
		name          string
		forceCache    bool
		items         []v2alpha1.DeleteItem
		manifests     map[string][]byte
		expected      []string
		expectedReads []string
	}{
		{
			name:          "known digest alias in another tag is deleted once",
			items:         []v2alpha1.DeleteItem{item("test.registry.io/"+colRepo+":v2@"+colDigestA, colDestRepo+":v2"), pinnedA, pinnedB},
			manifests:     map[string][]byte{colDestRepo + ":v2": colManA},
			expected:      colSorted(colDestRepo+"@"+colDigestA, colDestRepo+"@"+colDigestB),
			expectedReads: []string{colDestRepo + ":v2"},
		},
		{
			name:          "tag only alias is deleted once",
			items:         []v2alpha1.DeleteItem{pinnedA, pinnedB, item("test.registry.io/"+colRepo+":v2", colDestRepo+":v2")},
			manifests:     map[string][]byte{colDestRepo + ":v2": colManA},
			expected:      colSorted(colDestRepo+"@"+colDigestA, colDestRepo+"@"+colDigestB),
			expectedReads: []string{colDestRepo + ":v2"},
		},
		{
			name:          "unaliased tag is frozen to the manifest it points to",
			items:         []v2alpha1.DeleteItem{pinnedA, pinnedB, item("test.registry.io/"+colRepo+":v2", colDestRepo+":v2")},
			manifests:     map[string][]byte{colDestRepo + ":v2": colManC},
			expected:      colSorted(colDestRepo+"@"+colDigestA, colDestRepo+"@"+colDigestB, colDestRepo+"@"+colDigestC),
			expectedReads: []string{colDestRepo + ":v2"},
		},
		{
			name: "same digest pinned by two collision groups is deleted once",
			items: []v2alpha1.DeleteItem{
				pinnedA, pinnedB,
				item("test.registry.io/"+colRepo+":v3@"+colDigestA, colDestRepo+"@"+colDigestA),
				item("test.registry.io/"+colRepo+":v3@"+colDigestC, colDestRepo+"@"+colDigestC),
			},
			expected: colSorted(colDestRepo+"@"+colDigestA, colDestRepo+"@"+colDigestB, colDestRepo+"@"+colDigestC),
		},
		{
			name: "signature tags are resolved and deduplicated",
			items: []v2alpha1.DeleteItem{
				pinnedA, pinnedB,
				item("test.registry.io/"+colRepo+":"+colDigestTag(colDigestA)+".sig", colDestRepo+":"+colDigestTag(colDigestA)+".sig"),
				item("test.registry.io/"+colRepo+":"+colDigestTag(colDigestA)+".sig", colDestRepo+":"+colDigestTag(colDigestA)+".sig"),
				item("test.registry.io/"+colRepo+":"+colDigestTag(colDigestB)+".sig", colDestRepo+":"+colDigestTag(colDigestB)+".sig"),
			},
			manifests: map[string][]byte{
				colDestRepo + ":" + colDigestTag(colDigestA) + ".sig": colManSigA,
				colDestRepo + ":" + colDigestTag(colDigestB) + ".sig": colManSigB,
			},
			expected: colSorted(colDestRepo+"@"+colDigestA, colDestRepo+"@"+colDigestB, colDestRepo+"@"+colDigestSigA, colDestRepo+"@"+colDigestSigB),
			// each distinct tag reference is resolved once per invocation
			expectedReads: colSorted(
				colDestRepo+":"+colDigestTag(colDigestA)+".sig",
				colDestRepo+":"+colDigestTag(colDigestB)+".sig",
			),
		},
		{
			name:       "cache duplicates and cache aliases are deleted once",
			forceCache: true,
			items: []v2alpha1.DeleteItem{
				pinnedA, pinnedB,
				item("test.registry.io/"+colRepo+":v2@"+colDigestA, colDestRepo+":v2"),
				item("test.registry.io/"+colRepo+":v3", colDestRepo+":v3"),
			},
			manifests: map[string][]byte{
				colDestRepo + ":v2":                           colManA,
				colDestRepo + ":v3":                           colManA,
				colCacheRepo + ":" + colDigestTag(colDigestA): colManA,
				colCacheRepo + ":" + colDigestTag(colDigestB): colManB,
				colCacheRepo + ":v3":                          colManA,
			},
			expected: colSorted(colDestRepo+"@"+colDigestA, colDestRepo+"@"+colDigestB, colCacheRepo+"@"+colDigestA, colCacheRepo+"@"+colDigestB),
			expectedReads: colSorted(
				colDestRepo+":v2", colDestRepo+":v3",
				colCacheRepo+":"+colDigestTag(colDigestA),
				colCacheRepo+":"+colDigestTag(colDigestB),
				colCacheRepo+":v3",
			),
		},
		{
			name:       "a covered destination keeps its distinct cache task",
			forceCache: true,
			items:      []v2alpha1.DeleteItem{pinnedA, pinnedB, item("test.registry.io/"+colRepo+":v3", colDestRepo+":v3")},
			manifests: map[string][]byte{
				colDestRepo + ":v3":                           colManA,
				colCacheRepo + ":" + colDigestTag(colDigestA): colManA,
				colCacheRepo + ":" + colDigestTag(colDigestB): colManB,
				colCacheRepo + ":v3":                          colManC,
			},
			expected: colSorted(colDestRepo+"@"+colDigestA, colDestRepo+"@"+colDigestB, colCacheRepo+"@"+colDigestA, colCacheRepo+"@"+colDigestB, colCacheRepo+"@"+colDigestC),
			expectedReads: colSorted(
				colDestRepo+":v3",
				colCacheRepo+":"+colDigestTag(colDigestA),
				colCacheRepo+":"+colDigestTag(colDigestB),
				colCacheRepo+":v3",
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &resolvingManifest{manifests: tt.manifests}
			b := &recordingBatch{}
			di := newCollisionDeleteImages(newCollisionOpts(t, tt.forceCache, false), b, m, nil)

			require.NoError(t, di.DeleteRegistryImages(v2alpha1.DeleteImageList{Items: tt.items}))
			assert.Equal(t, 1, b.calls)
			assert.Equal(t, tt.expected, b.destinations())
			assert.Equal(t, len(tt.expected), b.schema.TotalAdditionalImages)
			if tt.expectedReads == nil {
				assert.Empty(t, m.lookedUp())
			} else {
				assert.Equal(t, tt.expectedReads, m.lookedUp())
			}
		})
	}
}

func TestDeleteRegistryImagesPreflightSystemContexts(t *testing.T) {
	opts := newCollisionOpts(t, true, false)
	remoteCtx, err := opts.DestImage.NewSystemContext()
	require.NoError(t, err)
	require.NotEqual(t, types.OptionalBoolTrue, remoteCtx.DockerInsecureSkipTLSVerify, "test needs TLS verification enabled for the destination")

	m := &resolvingManifest{manifests: map[string][]byte{
		colDestRepo + ":v2":                           colManA,
		colCacheRepo + ":" + colDigestTag(colDigestA): colManA,
		colCacheRepo + ":" + colDigestTag(colDigestB): colManB,
		colCacheRepo + ":v2":                          colManA,
	}}
	b := &recordingBatch{}
	di := newCollisionDeleteImages(opts, b, m, nil)
	require.NoError(t, di.DeleteRegistryImages(v2alpha1.DeleteImageList{Items: []v2alpha1.DeleteItem{
		{ImageName: "test.registry.io/" + colRepo + ":v1@" + colDigestA, ImageReference: colDestRepo + "@" + colDigestA, Type: v2alpha1.TypeGeneric},
		{ImageName: "test.registry.io/" + colRepo + ":v1@" + colDigestB, ImageReference: colDestRepo + "@" + colDigestB, Type: v2alpha1.TypeGeneric},
		{ImageName: "test.registry.io/" + colRepo + ":v2", ImageReference: colDestRepo + ":v2", Type: v2alpha1.TypeGeneric},
	}}))

	require.NotEmpty(t, m.lookups)
	for _, l := range m.lookups {
		if strings.HasPrefix(l.ref, colCacheRepo) {
			assert.Equal(t, types.OptionalBoolTrue, l.insecure, "cache lookup %s must skip TLS verification", l.ref)
		} else {
			assert.Equal(t, remoteCtx.DockerInsecureSkipTLSVerify, l.insecure, "destination lookup %s must use the destination TLS settings", l.ref)
		}
	}
}

func TestDeleteRegistryImagesPreflightFreezesResolvedDigests(t *testing.T) {
	m := &resolvingManifest{manifests: map[string][]byte{colDestRepo + ":v2": colManC}}
	b := &recordingBatch{}
	// Retag v2 once the work has been handed to the batch worker: the scheduled reference
	// must already be the digest observed by the preflight, not the mutable tag.
	b.beforeReturn = func() {
		m.mu.Lock()
		m.manifests[colDestRepo+":v2"] = colManA
		m.mu.Unlock()
	}
	di := newCollisionDeleteImages(newCollisionOpts(t, false, false), b, m, nil)

	require.NoError(t, di.DeleteRegistryImages(v2alpha1.DeleteImageList{Items: []v2alpha1.DeleteItem{
		{ImageName: "test.registry.io/" + colRepo + ":v1@" + colDigestA, ImageReference: colDestRepo + "@" + colDigestA, Type: v2alpha1.TypeGeneric},
		{ImageName: "test.registry.io/" + colRepo + ":v1@" + colDigestB, ImageReference: colDestRepo + "@" + colDigestB, Type: v2alpha1.TypeGeneric},
		{ImageName: "test.registry.io/" + colRepo + ":v2", ImageReference: colDestRepo + ":v2", Type: v2alpha1.TypeGeneric},
	}}))

	assert.Equal(t, colSorted(colDestRepo+"@"+colDigestA, colDestRepo+"@"+colDigestB, colDestRepo+"@"+colDigestC), b.destinations())
	for _, ref := range b.destinations() {
		assert.Contains(t, ref, "@", "scheduled reference %s in an affected repository must be a digest", ref)
	}
	assert.Equal(t, []string{colDestRepo + ":v2"}, m.lookedUp(), "tags must not be read again after deduplication")
}

func TestDeleteRegistryImagesPreflightFailureDispatchesNothing(t *testing.T) {
	m := &resolvingManifest{
		manifests: map[string][]byte{
			colDestRepo + ":v3":                           colManC,
			colCacheRepo + ":" + colDigestTag(colDigestA): colManA,
			colCacheRepo + ":v2":                          colManC,
			colCacheRepo + ":v3":                          colManC,
		},
		errs: map[string]error{
			colDestRepo + ":v2":                           errors.New("manifest unknown: manifest unknown"),
			colCacheRepo + ":" + colDigestTag(colDigestB): errors.New("unauthorized: authentication required"),
		},
	}
	b := &recordingBatch{}
	di := newCollisionDeleteImages(newCollisionOpts(t, true, false), b, m, nil)

	err := di.DeleteRegistryImages(v2alpha1.DeleteImageList{Items: []v2alpha1.DeleteItem{
		{ImageName: "test.registry.io/" + colRepo + ":v1@" + colDigestA, ImageReference: colDestRepo + "@" + colDigestA, Type: v2alpha1.TypeGeneric},
		{ImageName: "test.registry.io/" + colRepo + ":v1@" + colDigestB, ImageReference: colDestRepo + "@" + colDigestB, Type: v2alpha1.TypeGeneric},
		{ImageName: "test.registry.io/" + colRepo + ":v2", ImageReference: colDestRepo + ":v2", Type: v2alpha1.TypeGeneric},
		{ImageName: "test.registry.io/" + colRepo + ":v3", ImageReference: colDestRepo + ":v3", Type: v2alpha1.TypeGeneric},
		{ImageName: "test.registry.io/otherns/app:v1", ImageReference: colDeleteDestination + "/otherns/app:v1", Type: v2alpha1.TypeGeneric},
	}})

	require.Error(t, err)
	assert.ErrorContains(t, err, colDestRepo+":v2")
	assert.ErrorContains(t, err, "manifest unknown")
	assert.ErrorContains(t, err, colCacheRepo+":"+colDigestTag(colDigestB))
	assert.ErrorContains(t, err, "unauthorized")
	assert.ErrorContains(t, err, "no images were deleted")
	assert.Zero(t, b.calls, "no deletion may be dispatched when the preflight fails")
}

func TestDeleteRegistryImagesUnaffectedRepositoriesKeepLegacyTasks(t *testing.T) {
	m := &resolvingManifest{manifests: map[string][]byte{}}
	b := &recordingBatch{}
	di := newCollisionDeleteImages(newCollisionOpts(t, true, false), b, m, nil)

	require.NoError(t, di.DeleteRegistryImages(v2alpha1.DeleteImageList{Items: []v2alpha1.DeleteItem{
		{ImageName: "test.registry.io/otherns/app:v1@" + colDigestA, ImageReference: colDeleteDestination + "/otherns/app:v1", Type: v2alpha1.TypeGeneric},
		{ImageName: "test.registry.io/otherns/app:v2", ImageReference: colDeleteDestination + "/otherns/app:v2", Type: v2alpha1.TypeGeneric},
	}}))

	assert.Empty(t, m.lookedUp(), "repositories without digest references must not be read")
	assert.Equal(t, []v2alpha1.CopyImageSchema{
		{Origin: "test.registry.io/otherns/app:v1@" + colDigestA, Destination: colDeleteDestination + "/otherns/app:v1", Type: v2alpha1.TypeGeneric},
		{Origin: "test.registry.io/otherns/app:v1@" + colDigestA, Destination: "docker://" + colCacheFQDN + "/otherns/app:" + colDigestTag(colDigestA), Type: v2alpha1.TypeGeneric},
		{Origin: "test.registry.io/otherns/app:v2", Destination: colDeleteDestination + "/otherns/app:v2", Type: v2alpha1.TypeGeneric},
		{Origin: "test.registry.io/otherns/app:v2", Destination: "docker://" + colCacheFQDN + "/otherns/app:v2", Type: v2alpha1.TypeGeneric},
	}, b.schema.AllImages)
	assert.Equal(t, 4, b.schema.TotalAdditionalImages)
}

func TestDeleteRegistryImagesPinnedReferenceSafety(t *testing.T) {
	tests := []struct {
		name          string
		item          v2alpha1.DeleteItem
		expectedError string
	}{
		{
			name:          "pinned reference outside the delete destination",
			item:          v2alpha1.DeleteItem{ImageName: "test.registry.io/" + colRepo + ":v1@" + colDigestA, ImageReference: "docker://otherhost:5000/myregistry/" + colRepo + "@" + colDigestA, Type: v2alpha1.TypeGeneric},
			expectedError: "does not match values found",
		},
		{
			name:          "pinned release content reference at the wrong path",
			item:          v2alpha1.DeleteItem{ImageName: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@" + colDigestA, ImageReference: colDeleteDestination + "/wrong/path@" + colDigestA, Type: v2alpha1.TypeOCPReleaseContent},
			expectedError: "expected release content",
		},
		{
			name: "pinned release content reference at the expected path",
			item: v2alpha1.DeleteItem{ImageName: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@" + colDigestA, ImageReference: colDeleteDestination + "/openshift/release@" + colDigestA, Type: v2alpha1.TypeOCPReleaseContent},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &recordingBatch{}
			di := newCollisionDeleteImages(newCollisionOpts(t, false, false), b, &resolvingManifest{}, nil)
			err := di.DeleteRegistryImages(v2alpha1.DeleteImageList{Items: []v2alpha1.DeleteItem{tt.item}})
			if tt.expectedError != "" {
				assert.ErrorContains(t, err, tt.expectedError)
				assert.Empty(t, b.schema.AllImages)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, []string{tt.item.ImageReference}, b.destinations())
		})
	}
}

func TestDeleteRegistryImagesPreflightLeavesOtherRepositoriesUnchanged(t *testing.T) {
	m := &resolvingManifest{manifests: map[string][]byte{
		colDestRepo + ":v2":                           colManC,
		colCacheRepo + ":" + colDigestTag(colDigestA): colManA,
		colCacheRepo + ":" + colDigestTag(colDigestB): colManB,
		colCacheRepo + ":v2":                          colManC,
	}}
	b := &recordingBatch{}
	di := newCollisionDeleteImages(newCollisionOpts(t, true, false), b, m, nil)

	require.NoError(t, di.DeleteRegistryImages(v2alpha1.DeleteImageList{Items: []v2alpha1.DeleteItem{
		{ImageName: "test.registry.io/otherns/app:v1@" + colDigestA, ImageReference: colDeleteDestination + "/otherns/app:v1", Type: v2alpha1.TypeGeneric},
		{ImageName: "test.registry.io/" + colRepo + ":v1@" + colDigestA, ImageReference: colDestRepo + "@" + colDigestA, Type: v2alpha1.TypeGeneric},
		{ImageName: "test.registry.io/" + colRepo + ":v1@" + colDigestB, ImageReference: colDestRepo + "@" + colDigestB, Type: v2alpha1.TypeGeneric},
		{ImageName: "test.registry.io/" + colRepo + ":v2", ImageReference: colDestRepo + ":v2", Type: v2alpha1.TypeGeneric},
		{ImageName: "test.registry.io/otherns/app:v2", ImageReference: colDeleteDestination + "/otherns/app:v2", Type: v2alpha1.TypeGeneric},
	}}))

	for _, ref := range m.lookedUp() {
		assert.NotContains(t, ref, "otherns", "a repository without digest references must not be read")
	}
	var others []v2alpha1.CopyImageSchema
	for _, img := range b.schema.AllImages {
		if strings.Contains(img.Destination, "otherns") {
			others = append(others, img)
		}
	}
	assert.Equal(t, []v2alpha1.CopyImageSchema{
		{Origin: "test.registry.io/otherns/app:v1@" + colDigestA, Destination: colDeleteDestination + "/otherns/app:v1", Type: v2alpha1.TypeGeneric},
		{Origin: "test.registry.io/otherns/app:v1@" + colDigestA, Destination: "docker://" + colCacheFQDN + "/otherns/app:" + colDigestTag(colDigestA), Type: v2alpha1.TypeGeneric},
		{Origin: "test.registry.io/otherns/app:v2", Destination: colDeleteDestination + "/otherns/app:v2", Type: v2alpha1.TypeGeneric},
		{Origin: "test.registry.io/otherns/app:v2", Destination: "docker://" + colCacheFQDN + "/otherns/app:v2", Type: v2alpha1.TypeGeneric},
	}, others)
	assert.Equal(t, colSorted(
		colDeleteDestination+"/otherns/app:v1",
		"docker://"+colCacheFQDN+"/otherns/app:"+colDigestTag(colDigestA),
		colDeleteDestination+"/otherns/app:v2",
		"docker://"+colCacheFQDN+"/otherns/app:v2",
		colDestRepo+"@"+colDigestA, colDestRepo+"@"+colDigestB, colDestRepo+"@"+colDigestC,
		colCacheRepo+"@"+colDigestA, colCacheRepo+"@"+colDigestB, colCacheRepo+"@"+colDigestC,
	), b.destinations())
}

func TestDeleteRegistryImagesPreflightWithoutManifestClient(t *testing.T) {
	pinned := []v2alpha1.DeleteItem{
		{ImageName: "test.registry.io/" + colRepo + ":v1@" + colDigestA, ImageReference: colDestRepo + "@" + colDigestA, Type: v2alpha1.TypeGeneric},
		{ImageName: "test.registry.io/" + colRepo + ":v1@" + colDigestB, ImageReference: colDestRepo + "@" + colDigestB, Type: v2alpha1.TypeGeneric},
	}

	t.Run("digest references need no lookup", func(t *testing.T) {
		b := &recordingBatch{}
		di := newCollisionDeleteImages(newCollisionOpts(t, false, false), b, nil, nil)
		require.NoError(t, di.DeleteRegistryImages(v2alpha1.DeleteImageList{Items: pinned}))
		assert.Equal(t, colSorted(colDestRepo+"@"+colDigestA, colDestRepo+"@"+colDigestB), b.destinations())
	})

	t.Run("a tag that cannot be resolved stops the deletion", func(t *testing.T) {
		b := &recordingBatch{}
		di := newCollisionDeleteImages(newCollisionOpts(t, false, false), b, nil, nil)
		items := append(append([]v2alpha1.DeleteItem{}, pinned...),
			v2alpha1.DeleteItem{ImageName: "test.registry.io/" + colRepo + ":v2", ImageReference: colDestRepo + ":v2", Type: v2alpha1.TypeGeneric})
		err := di.DeleteRegistryImages(v2alpha1.DeleteImageList{Items: items})
		require.Error(t, err)
		assert.ErrorContains(t, err, colDestRepo+":v2")
		assert.ErrorContains(t, err, "no manifest client configured")
		assert.ErrorContains(t, err, "no images were deleted")
		assert.Zero(t, b.calls)
	})
}
