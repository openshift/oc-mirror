package artifactfilter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/opencontainers/go-digest"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/copy"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/signature"
	"go.podman.io/image/v5/transports/alltransports"
	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
)

// pushMultiArchWithAttestation pushes an amd64/arm64 index with one attestation
// entry and returns its reference and the two runnable digests.
func pushMultiArchWithAttestation(t *testing.T, ctx context.Context, host string) (string, []string) {
	t.Helper()

	ref := host + "/test/attestation:latest"
	tag, err := name.ParseReference(ref, name.Insecure)
	require.NoError(t, err)

	amd64, err := random.Image(256, 1)
	require.NoError(t, err)

	arm64, err := random.Image(256, 1)
	require.NoError(t, err)

	attestation, err := random.Image(64, 1)
	require.NoError(t, err)

	amd64Desc, err := partial_desc(amd64, "linux", "amd64")
	require.NoError(t, err)

	arm64Desc, err := partial_desc(arm64, "linux", "arm64")
	require.NoError(t, err)

	attestDesc, err := partial_desc(attestation, "unknown", "unknown")
	require.NoError(t, err)
	attestDesc.Annotations = map[string]string{
		"vnd.docker.reference.type": "attestation-manifest",
	}

	idx := mutate.AppendManifests(empty.Index,
		mutate.IndexAddendum{Add: amd64, Descriptor: amd64Desc},
		mutate.IndexAddendum{Add: arm64, Descriptor: arm64Desc},
		mutate.IndexAddendum{Add: attestation, Descriptor: attestDesc},
	)

	err = remote.WriteIndex(tag, idx, remote.WithContext(ctx))
	require.NoError(t, err)

	return ref, []string{amd64Desc.Digest.String(), arm64Desc.Digest.String()}
}

// pushMultiArchClean creates a manifest list with two real platform images
// and no attestation entries.
func pushMultiArchClean(t *testing.T, ctx context.Context, host string) string {
	t.Helper()

	ref := host + "/test/clean:latest"
	tag, err := name.ParseReference(ref, name.Insecure)
	require.NoError(t, err)

	amd64, err := random.Image(256, 1)
	require.NoError(t, err)

	arm64, err := random.Image(256, 1)
	require.NoError(t, err)

	amd64Desc, err := partial_desc(amd64, "linux", "amd64")
	require.NoError(t, err)

	arm64Desc, err := partial_desc(arm64, "linux", "arm64")
	require.NoError(t, err)

	idx := mutate.AppendManifests(empty.Index,
		mutate.IndexAddendum{Add: amd64, Descriptor: amd64Desc},
		mutate.IndexAddendum{Add: arm64, Descriptor: arm64Desc},
	)

	err = remote.WriteIndex(tag, idx, remote.WithContext(ctx))
	require.NoError(t, err)

	return ref
}

// pushSingleImage creates a single-arch image (not a manifest list).
func pushSingleImage(t *testing.T, ctx context.Context, host string) string {
	t.Helper()

	ref := host + "/test/single:latest"
	tag, err := name.ParseReference(ref, name.Insecure)
	require.NoError(t, err)

	img, err := random.Image(256, 1)
	require.NoError(t, err)

	err = remote.Write(tag, img, remote.WithContext(ctx))
	require.NoError(t, err)

	return ref
}

// pushAttestationOnly creates a manifest list holding nothing but an
// attestation entry.
func pushAttestationOnly(t *testing.T, ctx context.Context, host string) string {
	t.Helper()

	ref := host + "/test/attestation-only:latest"
	tag, err := name.ParseReference(ref, name.Insecure)
	require.NoError(t, err)

	attestation, err := random.Image(64, 1)
	require.NoError(t, err)

	attestDesc, err := partial_desc(attestation, "unknown", "unknown")
	require.NoError(t, err)
	attestDesc.Annotations = map[string]string{
		"vnd.docker.reference.type": "attestation-manifest",
	}

	idx := mutate.AppendManifests(empty.Index,
		mutate.IndexAddendum{Add: attestation, Descriptor: attestDesc},
	)

	err = remote.WriteIndex(tag, idx, remote.WithContext(ctx))
	require.NoError(t, err)

	return ref
}

// fixtureContext bounds a registry operation of t.
func fixtureContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	return ctx
}

// digestStrings renders selected digests for comparison against the digests
// returned by the push helpers.
func digestStrings(digests []digest.Digest) []string {
	out := make([]string, 0, len(digests))
	for _, d := range digests {
		out = append(out, d.String())
	}
	return out
}

func partial_desc(img v1.Image, os, arch string) (v1.Descriptor, error) {
	mt, err := img.MediaType()
	if err != nil {
		return v1.Descriptor{}, fmt.Errorf("getting image media type: %w", err)
	}
	digest, err := img.Digest()
	if err != nil {
		return v1.Descriptor{}, fmt.Errorf("getting image digest: %w", err)
	}
	size, err := img.Size()
	if err != nil {
		return v1.Descriptor{}, fmt.Errorf("getting image size: %w", err)
	}
	return v1.Descriptor{
		MediaType: mt,
		Digest:    digest,
		Size:      size,
		Platform: &v1.Platform{
			OS:           os,
			Architecture: arch,
		},
	}, nil
}

// recordingLogger captures the messages emitted by RetryCopyWithoutExcluded.
type recordingLogger struct {
	clog.PluggableLoggerInterface
	debug, warn []string
}

func (l *recordingLogger) Debug(msg string, val ...interface{}) {
	l.debug = append(l.debug, fmt.Sprintf(msg, val...))
}

func (l *recordingLogger) Warn(msg string, val ...interface{}) {
	l.warn = append(l.warn, fmt.Sprintf(msg, val...))
}

// TestRetryCopyWithoutExcluded checks when the source is inspected, when the
// single filtered retry runs and which errors are returned.
func TestRetryCopyWithoutExcluded(t *testing.T) {
	t.Parallel()
	budget := fixtureContext(t)
	s := httptest.NewServer(registry.New())
	defer s.Close()
	u, err := url.Parse(s.URL)
	require.NoError(t, err)
	host := u.Host

	sysCtx := &types.SystemContext{
		DockerInsecureSkipTLSVerify: types.OptionalBoolTrue,
	}
	withAttestation, realDigests := pushMultiArchWithAttestation(t, budget, host)
	withAttestationTag, err := name.NewTag(withAttestation, name.Insecure)
	require.NoError(t, err)
	inspected, err := remote.Head(withAttestationTag, remote.WithContext(budget))
	require.NoError(t, err)
	copyErr := errors.New("full copy failed")
	retryErr := errors.New("filtered copy failed")

	for _, test := range []struct {
		name        string
		cancelled   bool
		expired     bool
		ociLayout   bool
		ref         string // empty: nil reference, which panics if the source is inspected
		selection   copy.ImageListSelection
		copyErr     error
		retryErr    error
		wantDigests []string // digests of the single retry; nil means no retry
		wantErr     []error
		wantDebug   bool
	}{
		{name: "narrowed index retries once without attestations", ref: withAttestation, selection: copy.CopyAllImages, copyErr: copyErr, wantDigests: realDigests},
		{name: "failed retry returns both errors", ref: withAttestation, selection: copy.CopyAllImages, copyErr: copyErr, retryErr: retryErr, wantDigests: realDigests, wantErr: []error{copyErr, retryErr}},
		{name: "index without attestation keeps the error", ref: pushMultiArchClean(t, budget, host), selection: copy.CopyAllImages, copyErr: copyErr, wantErr: []error{copyErr}},
		{name: "attestation-only index keeps the error", ref: pushAttestationOnly(t, budget, host), selection: copy.CopyAllImages, copyErr: copyErr, wantErr: []error{copyErr}},
		{name: "single image keeps the error", ref: pushSingleImage(t, budget, host), selection: copy.CopyAllImages, copyErr: copyErr, wantErr: []error{copyErr}},
		{name: "inspection failure keeps the error and logs", ref: host + "/nonexistent/image:missing", selection: copy.CopyAllImages, copyErr: copyErr, wantErr: []error{copyErr}, wantDebug: true},
		{name: "explicit selection is not inspected", selection: copy.CopySpecificImages, copyErr: copyErr, wantErr: []error{copyErr}},
		{name: "non-docker source is not inspected", ociLayout: true, selection: copy.CopyAllImages, copyErr: copyErr, wantErr: []error{copyErr}},
		{name: "expired deadline is not inspected", expired: true, selection: copy.CopyAllImages, copyErr: copyErr, wantErr: []error{copyErr}},
		{name: "cancelled context is not inspected", cancelled: true, selection: copy.CopyAllImages, copyErr: copyErr, wantErr: []error{copyErr}},
		{name: "cancelled copy is not inspected", selection: copy.CopyAllImages, copyErr: fmt.Errorf("copy: %w", context.Canceled), wantErr: []error{context.Canceled}},
		{name: "timed out copy is not inspected", selection: copy.CopyAllImages, copyErr: fmt.Errorf("copy: %w", context.DeadlineExceeded), wantErr: []error{context.DeadlineExceeded}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(budget)
			defer cancel()
			if test.cancelled {
				cancel()
			}
			if test.expired {
				ctx, cancel = context.WithDeadline(budget, time.Now().Add(-time.Second))
				defer cancel()
			}
			var srcRef types.ImageReference
			switch {
			case test.ociLayout:
				parsed, err := alltransports.ParseImageName("oci:" + t.TempDir() + ":latest")
				require.NoError(t, err)
				srcRef = parsed
			case test.ref != "":
				parsed, err := alltransports.ParseImageName(consts.DockerProtocol + test.ref)
				require.NoError(t, err)
				srcRef = parsed
			}
			platforms := []copy.InstancePlatformFilter{{OS: "linux", Architecture: "amd64"}}
			options := &copy.Options{ImageListSelection: test.selection, SourceCtx: sysCtx, InstancePlatforms: platforms}
			var retries []copy.Options
			var retryRefs []types.ImageReference
			log := &recordingLogger{}

			err := RetryCopyWithoutExcluded(ctx, log, consts.DockerProtocol+test.ref, srcRef, options, test.copyErr, func(ref types.ImageReference, retry *copy.Options) error {
				retryRefs = append(retryRefs, ref)
				retries = append(retries, *retry)
				return test.retryErr
			})

			for _, want := range test.wantErr {
				require.ErrorIs(t, err, want)
			}
			if test.wantErr == nil {
				require.NoError(t, err)
			}
			require.Equal(t, test.selection, options.ImageListSelection, "options must not be modified")
			require.Equal(t, platforms, options.InstancePlatforms, "options must not be modified")
			if test.wantDigests == nil {
				require.Empty(t, retries)
				require.Empty(t, log.warn)
			} else {
				require.Len(t, retries, 1)
				assert.Equal(t, copy.CopySpecificImages, retries[0].ImageListSelection)
				assert.Equal(t, test.wantDigests, digestStrings(retries[0].Instances))
				assert.Nil(t, retries[0].InstancePlatforms)
				guarded, ok := retryRefs[0].(guardedReference)
				require.True(t, ok)
				assert.Equal(t, inspected.Digest.String(), guarded.expected.String())
				assert.Equal(t, srcRef.DockerReference(), guarded.DockerReference())
				assert.Equal(t, srcRef.PolicyConfigurationIdentity(), guarded.PolicyConfigurationIdentity())
				assert.Equal(t, srcRef.PolicyConfigurationNamespaces(), guarded.PolicyConfigurationNamespaces())
				require.Len(t, log.warn, 1)
				assert.Contains(t, log.warn[0], test.copyErr.Error())
			}
			if test.wantDebug {
				require.Len(t, log.debug, 1)
				assert.Contains(t, log.debug[0], "attestation check skipped for "+consts.DockerProtocol+test.ref)
			} else {
				assert.Empty(t, log.debug)
			}
		})
	}
}

type guardTestReference struct {
	types.ImageReference
	source types.ImageSource
	err    error
	system *types.SystemContext
	opened int
}

type afterGuardOpenReference struct {
	types.ImageReference
	afterOpen func(types.ImageSource)
}

//nolint:ireturn // implements types.ImageReference without wrapping the raw source
func (ref afterGuardOpenReference) NewImageSource(ctx context.Context, system *types.SystemContext) (types.ImageSource, error) {
	source, err := ref.ImageReference.NewImageSource(ctx, system)
	if err != nil {
		return nil, fmt.Errorf("opening guarded fixture: %w", err)
	}
	ref.afterOpen(source)
	return source, nil
}

//nolint:ireturn // implements types.ImageReference
func (ref *guardTestReference) NewImageSource(_ context.Context, system *types.SystemContext) (types.ImageSource, error) {
	ref.opened++
	ref.system = system
	return ref.source, ref.err
}

type guardTestSource struct {
	types.ImageSource
	data              []byte
	readErr, closeErr error
	closes            int
	afterRead         func()
}

func (source *guardTestSource) GetManifest(context.Context, *digest.Digest) ([]byte, string, error) {
	if source.afterRead != nil {
		source.afterRead()
	}
	return source.data, manifest.GuessMIMEType(source.data), source.readErr
}
func (source *guardTestSource) Close() error { source.closes++; return source.closeErr }

func TestGuardedReferenceOwnership(t *testing.T) {
	original, err := alltransports.ParseImageName("docker://localhost/synthetic:test")
	require.NoError(t, err)
	data := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[]}`)
	readErr := errors.New("read failed")
	openErr := errors.New("open failed")
	for _, mode := range []string{"success", "open", "nil-open", "read", "mismatch", "cancel-read", "precanceled"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			raw := &guardTestSource{data: data}
			ref := &guardTestReference{ImageReference: original, source: raw}
			guard := guardedReference{ImageReference: ref, expected: digest.FromBytes(data)}
			ref.err = map[string]error{"open": openErr, "nil-open": openErr}[mode]
			raw.readErr = map[string]error{"read": readErr}[mode]
			configureGuardCase(mode, &guard, ref, raw, cancel)
			system := &types.SystemContext{DockerInsecureSkipTLSVerify: types.OptionalBoolTrue}
			source, err := guard.NewImageSource(ctx, system)
			if mode == "success" {
				require.NoError(t, err)
				require.Same(t, raw, source)
				require.Zero(t, raw.closes)
				require.Same(t, system, ref.system)
				require.Equal(t, original.DockerReference(), guard.DockerReference())
				require.Equal(t, original.PolicyConfigurationIdentity(), guard.PolicyConfigurationIdentity())
				require.Equal(t, original.PolicyConfigurationNamespaces(), guard.PolicyConfigurationNamespaces())
				require.NoError(t, source.Close())
				require.Equal(t, 1, raw.closes)
				return
			}
			require.Error(t, err)
			require.Nil(t, source)
			wantCloses := 1
			if mode == "nil-open" || mode == "precanceled" {
				wantCloses = 0
			}
			require.Equal(t, wantCloses, raw.closes)
			if mode == "precanceled" {
				require.Zero(t, ref.opened)
				require.ErrorIs(t, err, context.Canceled)
			}
			if ref.err != nil {
				require.ErrorIs(t, err, ref.err)
			}
			if raw.readErr != nil {
				require.ErrorIs(t, err, readErr)
			}
		})
	}
}

func configureGuardCase(mode string, guard *guardedReference, ref *guardTestReference, raw *guardTestSource, cancel context.CancelFunc) {
	switch mode {
	case "nil-open":
		ref.source = nil
	case "mismatch":
		guard.expected = digest.FromString("other")
	case "cancel-read":
		raw.afterRead = cancel
	case "precanceled":
		cancel()
	}
}

func sparseCopyTarget(t *testing.T) (string, func() []byte) {
	t.Helper()
	var saved []byte
	var mutex sync.Mutex
	handler := registry.New()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPut && request.URL.Path == "/v2/target/manifests/test" {
			data, err := io.ReadAll(request.Body)
			if err != nil {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			mutex.Lock()
			saved = data
			mutex.Unlock()
			writer.Header().Set("Docker-Content-Digest", digest.FromBytes(data).String())
			writer.WriteHeader(http.StatusCreated)
			return
		}
		handler.ServeHTTP(writer, request)
	}))
	t.Cleanup(server.Close)
	return server.URL, func() []byte { mutex.Lock(); defer mutex.Unlock(); return append([]byte(nil), saved...) }
}

func TestGuardedFallbackRealCopy(t *testing.T) {
	for _, mode := range []string{"stable", "moved", "policy reject", "cached snapshot", "cached restored"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			sourceServer := httptest.NewServer(registry.New())
			defer sourceServer.Close()
			sourceURL, err := url.Parse(sourceServer.URL)
			require.NoError(t, err)
			sourceName, runnable := pushMultiArchWithAttestation(t, ctx, sourceURL.Host)
			tag, err := name.NewTag(sourceName, name.Insecure)
			require.NoError(t, err)
			index, err := remote.Get(tag, remote.WithContext(ctx))
			require.NoError(t, err)
			originalIndex, err := index.ImageIndex()
			require.NoError(t, err)
			original, err := alltransports.ParseImageName("docker://" + sourceName)
			require.NoError(t, err)
			destinationAddress, readPublished := sparseCopyTarget(t)
			destinationURL, err := url.Parse(destinationAddress)
			require.NoError(t, err)
			destination, err := alltransports.ParseImageName("docker://" + destinationURL.Host + "/target:test")
			require.NoError(t, err)
			policy := &signature.Policy{Default: signature.PolicyRequirements{signature.NewPRInsecureAcceptAnything()}}
			if mode == "policy reject" {
				policy.Transports = map[string]signature.PolicyTransportScopes{"docker": {sourceName: signature.PolicyRequirements{signature.NewPRReject()}}}
			}
			policyContext, err := signature.NewPolicyContext(policy)
			require.NoError(t, err)
			defer func() { require.NoError(t, policyContext.Destroy()) }()
			system := &types.SystemContext{DockerInsecureSkipTLSVerify: types.OptionalBoolTrue}
			originalError := errors.New("initial copy failed")
			var copied []byte
			err = RetryCopyWithoutExcluded(ctx, &recordingLogger{}, sourceName, original, &copy.Options{SourceCtx: system, DestinationCtx: system, ImageListSelection: copy.CopyAllImages, PreserveDigests: true}, originalError, func(selected types.ImageReference, options *copy.Options) error {
				if mode == "moved" {
					require.NoError(t, remote.WriteIndex(tag, empty.Index, remote.WithContext(ctx)))
				}
				if strings.HasPrefix(mode, "cached") {
					selected = afterGuardOpenReference{ImageReference: selected, afterOpen: func(source types.ImageSource) {
						require.Equal(t, original, source.Reference())
						changeTagAfterGuardOpen(t, ctx, tag, originalIndex, mode == "cached restored")
					}}
				}
				var copyErr error
				copied, copyErr = copy.Image(ctx, policyContext, destination, selected, options)
				if copyErr != nil {
					return fmt.Errorf("copying guarded fixture: %w", copyErr)
				}
				return nil
			})
			published := readPublished()
			if mode == "moved" || mode == "policy reject" {
				require.ErrorIs(t, err, originalError)
				require.Empty(t, published)
				if mode == "moved" {
					require.ErrorContains(t, err, "source index changed")
				} else {
					require.ErrorContains(t, err, "rejected")
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, index.Manifest, copied)
			require.Equal(t, index.Manifest, published)
			for _, instance := range runnable {
				child, parseErr := name.NewDigest(destinationURL.Host+"/target@"+instance, name.Insecure)
				require.NoError(t, parseErr)
				_, readErr := remote.Get(child, remote.WithContext(ctx))
				require.NoError(t, readErr)
			}
		})
	}
}

func changeTagAfterGuardOpen(t *testing.T, ctx context.Context, tag name.Tag, index v1.ImageIndex, restore bool) {
	t.Helper()
	require.NoError(t, remote.WriteIndex(tag, empty.Index, remote.WithContext(ctx)))
	if restore {
		require.NoError(t, remote.WriteIndex(tag, index, remote.WithContext(ctx)))
	}
}

func TestFallbackInspectionFailuresAndCancellation(t *testing.T) {
	original, err := alltransports.ParseImageName("docker://localhost/synthetic:test")
	require.NoError(t, err)
	index := testList(t, "", specv1.Descriptor{Digest: digest.FromString("runnable")}, specv1.Descriptor{Digest: digest.FromString("attestation"), Annotations: map[string]string{referenceTypeAnnotation: attestationManifestReferenceType}})
	data, err := index.Serialize()
	require.NoError(t, err)
	first := errors.New("initial copy failed")
	failure := errors.New("inspection failed")
	for _, mode := range []string{"open", "read", "parse", "close", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			source := &guardTestSource{data: data}
			reference := &guardTestReference{ImageReference: original, source: source}
			switch mode {
			case "open":
				reference.err = failure
			case "read":
				source.readErr = failure
			case "parse":
				source.data = []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":{}}`)
			case "close":
				source.closeErr = failure
			case "cancel":
				source.afterRead = cancel
			}
			logger := &recordingLogger{}
			callbacks := 0
			err := RetryCopyWithoutExcluded(ctx, logger, "synthetic", reference, &copy.Options{ImageListSelection: copy.CopyAllImages}, first, func(types.ImageReference, *copy.Options) error {
				callbacks++
				return nil
			})
			require.ErrorIs(t, err, first)
			require.Equal(t, 1, source.closes)
			require.Zero(t, callbacks, "inspection failure must not copy")
			require.Empty(t, logger.warn, "inspection failure must not warn about retrying")
			switch mode {
			case "open", "read", "close":
				require.ErrorIs(t, err, failure)
			case "cancel":
				require.ErrorIs(t, err, context.Canceled)
			}
		})
	}
}

func TestGuardedFallbackTagOnlyMirror(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(fmt.Sprintf("reject=%t", reject), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			var tagReads atomic.Int32
			handler := registry.New()
			mirror := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/manifests/latest") {
					tagReads.Add(1)
				}
				handler.ServeHTTP(writer, request)
			}))
			defer mirror.Close()
			mirrorURL, err := url.Parse(mirror.URL)
			require.NoError(t, err)
			sourceName, runnable := pushMultiArchWithAttestation(t, ctx, mirrorURL.Host)
			mirrorTag, err := name.NewTag(sourceName, name.Insecure)
			require.NoError(t, err)
			index, err := remote.Get(mirrorTag, remote.WithContext(ctx))
			require.NoError(t, err)
			var listenConfig net.ListenConfig
			closedPrimary, err := listenConfig.Listen(ctx, "tcp", "127.0.0.1:0")
			require.NoError(t, err)
			primary := closedPrimary.Addr().String()
			require.NoError(t, closedPrimary.Close())
			sourceName = primary + "/test/attestation:latest"
			configuration, err := os.CreateTemp(t.TempDir(), "registries-*.conf")
			require.NoError(t, err)
			_, err = fmt.Fprintf(configuration, `[[registry]]
prefix = "%s"
location = "%s"
insecure = true
[[registry.mirror]]
location = "%s"
insecure = true
pull-from-mirror = "tag-only"
`, primary, primary, mirrorURL.Host)
			closeErr := configuration.Close()
			require.NoError(t, err)
			require.NoError(t, closeErr)
			original, err := alltransports.ParseImageName("docker://" + sourceName)
			require.NoError(t, err)
			targetAddress, published := sparseCopyTarget(t)
			targetURL, err := url.Parse(targetAddress)
			require.NoError(t, err)
			target, err := alltransports.ParseImageName("docker://" + targetURL.Host + "/target:test")
			require.NoError(t, err)
			policy := &signature.Policy{Default: signature.PolicyRequirements{signature.NewPRInsecureAcceptAnything()}}
			if reject {
				policy.Transports = map[string]signature.PolicyTransportScopes{"docker": {sourceName: signature.PolicyRequirements{signature.NewPRReject()}}}
			}
			policyContext, err := signature.NewPolicyContext(policy)
			require.NoError(t, err)
			defer func() { require.NoError(t, policyContext.Destroy()) }()
			system := &types.SystemContext{SystemRegistriesConfPath: configuration.Name(), DockerInsecureSkipTLSVerify: types.OptionalBoolTrue}
			first := errors.New("initial copy failed")
			before := tagReads.Load()
			callbacks := 0
			err = RetryCopyWithoutExcluded(ctx, &recordingLogger{}, sourceName, original, &copy.Options{SourceCtx: system, DestinationCtx: system, ImageListSelection: copy.CopyAllImages, PreserveDigests: true}, first, func(selected types.ImageReference, options *copy.Options) error {
				callbacks++
				require.Same(t, system, options.SourceCtx)
				require.Equal(t, original.PolicyConfigurationIdentity(), selected.PolicyConfigurationIdentity())
				_, copyErr := copy.Image(ctx, policyContext, target, selected, options)
				if copyErr != nil {
					return fmt.Errorf("copying tag-only mirror fixture: %w", copyErr)
				}
				return nil
			})
			require.Equal(t, 1, callbacks)
			if reject {
				require.ErrorIs(t, err, first)
				require.ErrorContains(t, err, "rejected")
				require.Empty(t, published())
				return
			}
			require.NoError(t, err, "tag-only mirror must work with an unavailable primary")
			require.GreaterOrEqual(t, tagReads.Load()-before, int32(2), "inspection and guarded copy must use the original tag routing")
			require.Equal(t, index.Manifest, published())
			for _, childDigest := range runnable {
				child, parseErr := name.NewDigest(targetURL.Host+"/target@"+childDigest, name.Insecure)
				require.NoError(t, parseErr)
				image, readErr := remote.Image(child, remote.WithContext(ctx))
				require.NoError(t, readErr)
				_, readErr = image.RawConfigFile()
				require.NoError(t, readErr)
				layers, readErr := image.Layers()
				require.NoError(t, readErr)
				for _, layer := range layers {
					content, openErr := layer.Compressed()
					require.NoError(t, openErr)
					_, readErr = io.Copy(io.Discard, content)
					require.NoError(t, content.Close())
					require.NoError(t, readErr)
				}
			}
		})
	}
}
