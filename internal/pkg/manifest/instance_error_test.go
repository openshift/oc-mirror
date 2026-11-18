package manifest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docker/distribution/registry/api/errcode"
	v2 "github.com/docker/distribution/registry/api/v2"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/transports/alltransports"
	"go.podman.io/image/v5/types"
)

// provenanceRegistry serves an empty OCI index as synthetic:test and answers
// MANIFEST_UNKNOWN for child, or 503 when childUnavailable is set. Once
// failParent is set, the index also answers MANIFEST_UNKNOWN.
type provenanceRegistry struct {
	t                             *testing.T
	child                         digest.Digest
	childUnavailable              bool
	failParent                    atomic.Bool
	parentRequests, childRequests atomic.Int32
}

func (registry *provenanceRegistry) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/v2/":
		writer.WriteHeader(http.StatusOK)
	case "/v2/synthetic/manifests/test":
		registry.parentRequests.Add(1)
		if registry.failParent.Load() {
			registry.manifestUnknown(writer)
			return
		}
		registry.reply(writer, http.StatusOK, "application/vnd.oci.image.index.v1+json", `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[]}`)
	case "/v2/synthetic/manifests/" + registry.child.String():
		registry.childRequests.Add(1)
		if registry.childUnavailable {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		registry.manifestUnknown(writer)
	default:
		writer.WriteHeader(http.StatusNotFound)
	}
}

func (registry *provenanceRegistry) manifestUnknown(writer http.ResponseWriter) {
	registry.reply(writer, http.StatusNotFound, "application/json", `{"errors":[{"code":"MANIFEST_UNKNOWN","message":"synthetic missing manifest"}]}`)
}

func (registry *provenanceRegistry) reply(writer http.ResponseWriter, status int, contentType, body string) {
	writer.Header().Set("Content-Type", contentType)
	writer.WriteHeader(status)
	if _, err := writer.Write([]byte(body)); err != nil {
		registry.t.Errorf("writing synthetic registry response: %v", err)
	}
}

func TestInstanceReadProvenanceHTTP(t *testing.T) {
	for _, test := range []struct {
		name                         string
		failParent, childUnavailable bool
		wantManifestUnknown          bool
		wantChildRequests            int32
	}{
		{name: "parent-open", failParent: true, wantManifestUnknown: true},
		{name: "child-read", wantManifestUnknown: true, wantChildRequests: 1},
		{name: "child-503", childUnavailable: true, wantChildRequests: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			registry := &provenanceRegistry{t: t, child: digest.FromString("synthetic-attestation"), childUnavailable: test.childUnavailable}
			server := httptest.NewServer(registry)
			defer server.Close()
			reference := "docker://" + strings.TrimPrefix(server.URL, "http://") + "/synthetic:test"
			system := &types.SystemContext{DockerInsecureSkipTLSVerify: types.OptionalBoolTrue}
			_, _, err := (Manifest{}).ImageManifest(ctx, system, reference, nil)
			require.NoError(t, err)
			registry.failParent.Store(test.failParent)
			_, _, err = (Manifest{}).ImageManifest(ctx, system, reference, &registry.child)
			require.Error(t, err)
			var coded errcode.ErrorCoder
			require.Equal(t, test.wantManifestUnknown, errors.As(err, &coded) && coded.ErrorCode() == v2.ErrorCodeManifestUnknown, "registry error code: %v", err)
			require.Equal(t, !test.failParent, InstanceReadError(err, reference, registry.child) != nil)
			require.Nil(t, InstanceReadError(err, reference+"wrong", registry.child))
			require.Nil(t, InstanceReadError(err, reference, digest.FromString("other")))
			require.EqualValues(t, 2, registry.parentRequests.Load())
			require.Equal(t, test.wantChildRequests, registry.childRequests.Load())
		})
	}
}

type manifestTestReference struct {
	types.ImageReference
	source types.ImageSource
	err    error
}

//nolint:ireturn // implements types.ImageReference
func (reference manifestTestReference) NewImageSource(context.Context, *types.SystemContext) (types.ImageSource, error) {
	return reference.source, reference.err
}

type manifestTestSource struct {
	types.ImageSource
	readErr, closeErr error
	closes            int
}

func (source *manifestTestSource) GetManifest(context.Context, *digest.Digest) ([]byte, string, error) {
	return []byte(`{}`), "", source.readErr
}
func (source *manifestTestSource) Close() error { source.closes++; return source.closeErr }

func TestInstanceReadProvenanceBoundaries(t *testing.T) {
	missing := v2.ErrorCodeManifestUnknown.WithDetail(nil)
	closeErr := errors.New("close failed")
	docker, err := alltransports.ParseImageName("docker://localhost/synthetic:test")
	require.NoError(t, err)
	oci, err := alltransports.ParseImageName("oci:" + t.TempDir())
	require.NoError(t, err)
	child := digest.FromString("child")
	invalid := digest.Digest("invalid")
	for _, test := range []struct {
		name                       string
		ref                        types.ImageReference
		instance                   *digest.Digest
		openErr, readErr, closeErr error
		wantEligible               bool
	}{
		{name: "child", ref: docker, instance: &child, readErr: missing, wantEligible: true},
		{name: "parent", ref: docker, readErr: missing},
		{name: "open", ref: docker, instance: &child, openErr: missing},
		{name: "open and close", ref: docker, instance: &child, openErr: missing, closeErr: closeErr},
		{name: "child and close", ref: docker, instance: &child, readErr: missing, closeErr: closeErr},
		{name: "success and close", ref: docker, instance: &child, closeErr: closeErr},
		{name: "non docker", ref: oci, instance: &child, readErr: missing},
		{name: "invalid digest", ref: docker, instance: &invalid, readErr: missing},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := &manifestTestSource{readErr: test.readErr, closeErr: test.closeErr}
			_, _, err := readManifest(context.Background(), nil, "synthetic", test.instance, manifestTestReference{ImageReference: test.ref, source: source, err: test.openErr})
			require.Error(t, err)
			require.Equal(t, test.wantEligible, InstanceReadError(err, "synthetic", child) != nil)
			require.Equal(t, 1, source.closes)
			for _, cause := range []error{test.openErr, test.readErr, test.closeErr} {
				if cause != nil {
					require.ErrorIs(t, err, cause)
				}
			}
		})
	}
	var nilSource *manifestTestSource
	for _, source := range []types.ImageSource{nil, nilSource} {
		_, _, err := readManifest(context.Background(), nil, "synthetic", &child, manifestTestReference{ImageReference: docker, source: source, err: missing})
		require.ErrorIs(t, err, missing)
		require.Nil(t, InstanceReadError(err, "synthetic", child))
		_, _, err = readManifest(context.Background(), nil, "synthetic", &child, manifestTestReference{ImageReference: docker, source: source})
		require.Error(t, err)
		require.Nil(t, InstanceReadError(err, "synthetic", child))
	}
	require.Nil(t, InstanceReadError(missing, "synthetic", child))
	_, _, err = (Manifest{}).ImageManifest(context.Background(), nil, "invalid transport", &child)
	require.Error(t, err)
	require.Nil(t, InstanceReadError(err, "invalid transport", child))
}

func TestInstanceReadRejectsOuterMultipleCauses(t *testing.T) {
	instance := digest.FromString("child")
	marker := &InstanceManifestError{Source: "docker://localhost/synthetic:test", Instance: instance, Err: v2.ErrorCodeManifestUnknown.WithDetail(nil)}
	require.NotNil(t, InstanceReadError(fmt.Errorf("singular: %w", marker), marker.Source, instance))
	for _, multiple := range []error{
		errors.Join(marker),
		errors.Join(marker, errors.New("503 Service Unavailable")),
		errors.Join(marker, context.Canceled),
		errors.Join(marker, context.DeadlineExceeded),
		fmt.Errorf("multiple: %w; %w", marker, errors.New("unrelated failure")),
		&InstanceManifestError{Source: marker.Source, Instance: instance, Err: errors.Join(marker.Err, context.Canceled)},
	} {
		for _, outer := range []error{multiple, fmt.Errorf("wrapped: %w", multiple)} {
			require.Nil(t, InstanceReadError(outer, marker.Source, instance))
		}
	}
}
