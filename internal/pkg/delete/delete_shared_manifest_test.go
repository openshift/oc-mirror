package delete

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/batch"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func TestDeleteRegistryImagesCollapsesSharedManifest(t *testing.T) {
	digestA := mustDigest(t, "a")
	digestB := mustDigest(t, "b")
	release := "docker://localhost:5000/myregistry/openshift/release:"
	origin := "quay.io/ocp/release@sha256:" + strings.Repeat("c", 64)

	tests := []struct {
		name           string
		items          []v2alpha1.DeleteItem
		resolve        func(ref string) (digest.Digest, error)
		forceCache     bool
		wantDest       []string
		wantRelease    int
		wantAdditional int
		forbidLookup   bool
	}{
		{
			name: "two tags of one manifest",
			items: []v2alpha1.DeleteItem{
				releaseItem(origin, release+"4.21.29-comp-a"),
				releaseItem(origin, release+"4.21.36-comp-b"),
			},
			resolve:     func(string) (digest.Digest, error) { return digestA, nil },
			wantDest:    []string{releaseRepo() + "@" + digestA.String()},
			wantRelease: 1,
		},
		{
			name: "different manifests stay separate",
			items: []v2alpha1.DeleteItem{
				releaseItem(origin, release+"4.21.29-comp-a"),
				releaseItem(origin, release+"4.21.29-comp-b"),
			},
			resolve: func(ref string) (digest.Digest, error) {
				if strings.HasSuffix(ref, "comp-b") {
					return digestB, nil
				}
				return digestA, nil
			},
			wantDest: []string{
				releaseRepo() + "@" + digestA.String(),
				releaseRepo() + "@" + digestB.String(),
			},
			wantRelease: 2,
		},
		{
			name: "same digest in another repository stays separate",
			items: []v2alpha1.DeleteItem{
				releaseItem(origin, release+"v1"),
				releaseItem(origin, "docker://localhost:5000/myregistry/other/app:v1"),
			},
			resolve: func(string) (digest.Digest, error) { return digestA, nil },
			wantDest: []string{
				releaseRepo() + "@" + digestA.String(),
				"docker://localhost:5000/myregistry/other/app@" + digestA.String(),
			},
			wantRelease: 2,
		},
		{
			name: "same tag with different explicit digests is not dropped",
			items: []v2alpha1.DeleteItem{
				releaseItem(origin, release+"v1@"+digestA.String()),
				releaseItem("quay.io/ocp/release@"+digestB.String(), release+"v1@"+digestB.String()),
			},
			forbidLookup: true,
			wantDest: []string{
				releaseRepo() + "@" + digestA.String(),
				releaseRepo() + "@" + digestB.String(),
			},
			wantRelease: 2,
		},
		{
			name: "tag plus digest does not hide the tag that names the same manifest",
			items: []v2alpha1.DeleteItem{
				releaseItem(origin, release+"v1@"+digestA.String()),
				releaseItem(origin, release+"v1"),
			},
			resolve:     func(string) (digest.Digest, error) { return digestA, nil },
			wantDest:    []string{releaseRepo() + "@" + digestA.String()},
			wantRelease: 1,
		},
		{
			name: "tag alias collapses only into the digest it points at",
			items: []v2alpha1.DeleteItem{
				releaseItem(origin, release+"v1"),
				releaseItem("quay.io/ocp/release@"+digestB.String(), release+"v1@"+digestB.String()),
			},
			resolve: func(string) (digest.Digest, error) { return digestA, nil },
			wantDest: []string{
				releaseRepo() + "@" + digestA.String(),
				releaseRepo() + "@" + digestB.String(),
			},
			wantRelease: 2,
		},
		{
			name: "unresolved tags are deleted as written and an origin digest is not reused",
			items: []v2alpha1.DeleteItem{
				releaseItem(origin, release+"4.21.29-comp-a"),
				releaseItem(origin, release+"4.21.36-comp-b"),
				releaseItem(origin, release+"4.21.36-comp-b"),
			},
			resolve: func(string) (digest.Digest, error) { return "", fmt.Errorf("registry unavailable") },
			wantDest: []string{
				release + "4.21.29-comp-a",
				release + "4.21.36-comp-b",
			},
			wantRelease: 2,
		},
		{
			name: "force cache collapses remote and cache independently",
			items: []v2alpha1.DeleteItem{
				releaseItem(origin, release+"4.21.29-comp-a"),
				releaseItem(origin, release+"4.21.36-comp-b"),
			},
			resolve:    func(string) (digest.Digest, error) { return digestA, nil },
			forceCache: true,
			wantDest: []string{
				releaseRepo() + "@" + digestA.String(),
				"docker://localhost:8888/openshift/release@" + digestA.String(),
			},
			wantRelease: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured v2alpha1.CollectorSchema
			di := &DeleteImages{
				Log: clog.New("trace"),
				Opts: mirror.CopyOptions{
					Global: &mirror.GlobalOptions{
						DeleteDestination: "docker://localhost:5000/myregistry",
						ForceCacheDelete:  tt.forceCache,
					},
				},
				Batch:            &mockBatch{capturedSchema: &captured},
				LocalStorageFQDN: "localhost:8888",
			}
			if tt.forbidLookup {
				di.resolveManifest = func(context.Context, string, bool) (digest.Digest, error) {
					t.Fatal("digest-qualified references must not be resolved from a tag")
					return "", nil
				}
			} else {
				di.resolveManifest = func(_ context.Context, ref string, _ bool) (digest.Digest, error) {
					return tt.resolve(ref)
				}
			}

			err := di.DeleteRegistryImages(v2alpha1.DeleteImageList{Items: tt.items})
			require.NoError(t, err)
			got := make([]string, 0, len(captured.AllImages))
			for _, img := range captured.AllImages {
				got = append(got, img.Destination)
			}
			require.Equal(t, tt.wantDest, got)
			require.Equal(t, tt.wantRelease, captured.TotalReleaseImages)
			require.Equal(t, tt.wantAdditional, captured.TotalAdditionalImages)
		})
	}
}

func TestLookupTimeoutIsPerReference(t *testing.T) {
	digestA := mustDigest(t, "a")
	release := "docker://localhost:5000/myregistry/openshift/release:"
	origin := "quay.io/ocp/release:v1"
	var calls int
	var firstDone <-chan struct{}
	var captured v2alpha1.CollectorSchema
	di := &DeleteImages{
		Log: clog.New("trace"),
		Opts: mirror.CopyOptions{
			Global: &mirror.GlobalOptions{
				DeleteDestination: "docker://localhost:5000/myregistry",
				CommandTimeout:    30 * time.Second,
			},
		},
		Batch: &mockBatch{capturedSchema: &captured},
		resolveManifest: func(ctx context.Context, ref string, _ bool) (digest.Digest, error) {
			calls++
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			require.Greater(t, time.Until(deadline), 20*time.Second)
			if strings.HasSuffix(ref, "comp-a") {
				firstDone = ctx.Done()
				return "", context.DeadlineExceeded
			}
			require.NotNil(t, firstDone)
			select {
			case <-firstDone:
			default:
				t.Fatal("first lookup context is still active")
			}
			require.NoError(t, ctx.Err())
			return digestA, nil
		},
	}

	err := di.DeleteRegistryImages(v2alpha1.DeleteImageList{Items: []v2alpha1.DeleteItem{
		releaseItem(origin, release+"comp-a"),
		releaseItem(origin, release+"comp-b"),
	}})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, []string{
		release + "comp-a",
		releaseRepo() + "@" + digestA.String(),
	}, scheduledDestinations(captured))
}

func TestDeleteSharedDestinationManifest(t *testing.T) {
	srv := httptest.NewServer(registry.New())
	t.Cleanup(srv.Close)
	registryURL, err := url.Parse(srv.URL)
	require.NoError(t, err)
	host := registryURL.Host

	shared := mustImage(t)
	other := mustImage(t)
	sharedDigest := imageDigest(t, shared)
	otherDigest := imageDigest(t, other)
	pushImage(t, host, "mirror/openshift/release:4.21.29-comp-a", shared)
	pushImage(t, host, "mirror/openshift/release:4.21.36-comp-b", shared)
	pushImage(t, host, "mirror/openshift/release:4.21.29-other", other)
	pushImage(t, host, "mirror/otherns/app:v1", shared)
	pushImage(t, host, "mirror/kept/app:v1", shared)

	opts := insecureDeleteOptions(t, "docker://"+host+"/mirror")
	di := newRegistryDeleter(t, opts, 2)
	recorded := &recordingBatch{next: di.Batch}
	di.Batch = recorded

	aliasA := "docker://" + host + "/mirror/openshift/release:4.21.29-comp-a"
	aliasB := "docker://" + host + "/mirror/openshift/release:4.21.36-comp-b"
	otherRef := "docker://" + host + "/mirror/openshift/release:4.21.29-other"
	otherRepo := "docker://" + host + "/mirror/otherns/app:v1"
	kept := "docker://" + host + "/mirror/kept/app:v1"

	err = di.DeleteRegistryImages(v2alpha1.DeleteImageList{Items: []v2alpha1.DeleteItem{
		releaseItem("quay.io/ocp/release:4.21.29-comp-a", aliasA),
		releaseItem("quay.io/ocp/release:4.21.36-comp-b", aliasB),
		releaseItem("quay.io/ocp/release:4.21.29-other", otherRef),
		{ImageName: "quay.io/example/app:v1", ImageReference: otherRepo, Type: v2alpha1.TypeGeneric},
	}})
	require.NoError(t, err)

	// The registry fixture deletes a manifest by the requested reference and does not
	// clear other tags. The digest entry is removed, so absence is checked by digest.
	destinations := scheduledDestinations(recorded.schema)
	require.Len(t, destinations, 3)
	require.NotContains(t, destinations, aliasA)
	require.NotContains(t, destinations, aliasB)
	require.NotContains(t, destinations, otherRef)
	require.NotContains(t, destinations, otherRepo)
	require.False(t, manifestDigestExists(t, host, "mirror/openshift/release", sharedDigest))
	require.False(t, manifestDigestExists(t, host, "mirror/openshift/release", otherDigest))
	require.False(t, manifestDigestExists(t, host, "mirror/otherns/app", sharedDigest))
	require.True(t, manifestDigestExists(t, host, "mirror/kept/app", sharedDigest))
	require.True(t, imageExists(t, opts, kept), "repository omitted from the plan remains")
}

func TestDeleteMissingTagReportsError(t *testing.T) {
	srv := httptest.NewServer(registry.New())
	t.Cleanup(srv.Close)
	registryURL, err := url.Parse(srv.URL)
	require.NoError(t, err)
	host := registryURL.Host

	pushImage(t, host, "mirror/ns/present:v1", mustImage(t))
	opts := insecureDeleteOptions(t, "docker://"+host+"/mirror")
	logs := t.TempDir()
	opts.Global.WorkingDir = logs
	di := newRegistryDeleter(t, opts, 2)
	di.Batch = batch.New("channel", clog.New("info"), logs, mirror.New(mirror.NewMirrorCopy(), mirror.NewMirrorDelete()), 2, "missing", nil)

	present := "docker://" + host + "/mirror/ns/present:v1"
	missing := "docker://" + host + "/mirror/ns/missing:v1"
	recorded := &recordingBatch{next: di.Batch}
	di.Batch = recorded
	err = di.DeleteRegistryImages(v2alpha1.DeleteImageList{Items: []v2alpha1.DeleteItem{
		{ImageName: "quay.io/example/app:missing", ImageReference: missing, Type: v2alpha1.TypeGeneric},
		{ImageName: "quay.io/example/app:present", ImageReference: present, Type: v2alpha1.TypeGeneric},
	}})
	require.Error(t, err)

	body, readErr := os.ReadFile(filepath.Join(logs, "mirroring_errors_missing.txt"))
	require.NoError(t, readErr)
	require.Contains(t, string(body), "may not exist")
	require.Contains(t, string(body), "app:missing")
	require.NotContains(t, string(body), "app:present")

	destinations := scheduledDestinations(recorded.schema)
	require.Contains(t, destinations, missing, "a tag that cannot be resolved is still deleted")
	require.NotContains(t, destinations, present)
	require.True(t, strings.Contains(strings.Join(destinations, "\n"), "/mirror/ns/present@sha256:"), "the present image is still scheduled")
}

type recordingBatch struct {
	next   batch.BatchInterface
	schema v2alpha1.CollectorSchema
}

func (r *recordingBatch) Worker(ctx context.Context, schema v2alpha1.CollectorSchema, opts mirror.CopyOptions) (v2alpha1.CollectorSchema, error) {
	r.schema = schema
	return r.next.Worker(ctx, schema, opts)
}

func scheduledDestinations(schema v2alpha1.CollectorSchema) []string {
	destinations := make([]string, 0, len(schema.AllImages))
	for _, img := range schema.AllImages {
		destinations = append(destinations, img.Destination)
	}
	return destinations
}

func releaseItem(name, ref string) v2alpha1.DeleteItem {
	return v2alpha1.DeleteItem{ImageName: name, ImageReference: ref, Type: v2alpha1.TypeOCPReleaseContent}
}

func releaseRepo() string {
	return "docker://localhost:5000/myregistry/openshift/release"
}

func mustDigest(t *testing.T, char string) digest.Digest {
	t.Helper()
	dgst, err := digest.Parse("sha256:" + strings.Repeat(char, 64))
	require.NoError(t, err)
	return dgst
}

func mustImage(t *testing.T) v1.Image { //nolint:ireturn // as expected by go-containerregistry
	t.Helper()
	img, err := random.Image(256, 1)
	require.NoError(t, err)
	return img
}

func imageDigest(t *testing.T, img v1.Image) string {
	t.Helper()
	dgst, err := img.Digest()
	require.NoError(t, err)
	return dgst.String()
}

func manifestDigestExists(t *testing.T, host, repository, dgst string) bool {
	t.Helper()
	ref, err := name.NewDigest(host+"/"+repository+"@"+dgst, name.Insecure)
	require.NoError(t, err)
	_, err = remote.Head(ref)
	if err == nil {
		return true
	}
	var transportErr *transport.Error
	if errors.As(err, &transportErr) && transportErr.StatusCode == http.StatusNotFound {
		return false
	}
	require.NoError(t, err)
	return false
}

func TestSkipTLSVerifyMatchesCacheHost(t *testing.T) {
	di := DeleteImages{LocalStorageFQDN: "localhost:8888"}
	require.True(t, di.skipTLSVerify("docker://example.com/ns/app:v1", true))
	require.True(t, di.skipTLSVerify("docker://localhost:8888/openshift/release:v1", false))
	require.False(t, di.skipTLSVerify("docker://mirror-localhost:8888/ns/app:v1", false))
	require.False(t, di.skipTLSVerify("docker://example.com/ns/app:v1", false))
}

func pushImage(t *testing.T, host, repoTag string, img v1.Image) {
	t.Helper()
	ref, err := name.NewTag(host+"/"+repoTag, name.Insecure)
	require.NoError(t, err)
	require.NoError(t, remote.Write(ref, img))
}

func insecureDeleteOptions(t *testing.T, deleteDestination string) mirror.CopyOptions {
	t.Helper()
	global := &mirror.GlobalOptions{
		SecurePolicy:      false,
		WorkingDir:        t.TempDir(),
		DeleteDestination: deleteDestination,
	}
	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	srcFlags, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	dstFlags, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()
	require.NoError(t, srcFlags.Set("src-tls-verify", "false"))
	require.NoError(t, dstFlags.Set("dest-tls-verify", "false"))
	return mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Function:            string(mirror.DeleteMode),
		LocalStorageFQDN:    "localhost:8888",
	}
}

func newRegistryDeleter(t *testing.T, opts mirror.CopyOptions, workers uint) *DeleteImages {
	t.Helper()
	log := clog.New("info")
	worker := batch.New("channel", log, t.TempDir(), mirror.New(mirror.NewMirrorCopy(), mirror.NewMirrorDelete()), workers, "shared", nil)
	created := New(log, opts, worker, &mockBlobs{}, v2alpha1.ImageSetConfiguration{}, manifest.New(log), t.TempDir(), &mockSignatureHandler{})
	di, ok := created.(*DeleteImages)
	require.True(t, ok)
	return di
}

func imageExists(t *testing.T, opts mirror.CopyOptions, ref string) bool {
	t.Helper()
	ok, err := mirror.New(mirror.NewMirrorCopy(), mirror.NewMirrorDelete()).Check(context.Background(), ref, &opts, false)
	require.NoError(t, err)
	return ok
}
