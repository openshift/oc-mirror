package integration_test

import (
	"archive/tar"
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/openshift/oc-mirror/tests/integration/pkg/ocmirror"
	localregistry "github.com/openshift/oc-mirror/tests/integration/pkg/registry"
)

//go:embed testdata/registry-config-sparse.yaml
var attestationArchiveRegistryConfig []byte

func TestAttestationArchiveCLI(t *testing.T) {
	for _, scenario := range []struct {
		name         string
		attestation  bool
		inaccessible bool
		single       bool
		sparse       bool
	}{
		{name: "available attestation", attestation: true},
		{name: "inaccessible attestation", attestation: true, inaccessible: true},
		{name: "clean index"},
		{name: "single image", single: true},
		{name: "explicit sparse platform", sparse: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			home := t.TempDir()
			for key, value := range map[string]string{
				"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, ".config"),
				"XDG_CACHE_HOME": filepath.Join(home, ".cache"), "XDG_RUNTIME_DIR": filepath.Join(home, "run"),
				"DOCKER_CONFIG": filepath.Join(home, ".docker"), "REGISTRY_AUTH_FILE": "", "OC_MIRROR_CACHE": "",
			} {
				t.Setenv(key, value)
			}
			archiveRequireNoError(t, os.Unsetenv("REGISTRY_AUTH_FILE"))
			archiveRequireNoError(t, os.MkdirAll(filepath.Join(home, "run"), 0o700))
			amd64 := syntheticArchiveImage(t, "amd64")
			arm64 := syntheticArchiveImage(t, "arm64")
			attestation := syntheticArchiveImage(t, "unknown")
			attestationDigest, err := attestation.Digest()
			archiveRequireNoError(t, err)
			var blocked atomic.Bool
			var attestationRequests atomic.Int64
			handler := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
			source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if strings.Contains(request.URL.Path, "/manifests/"+attestationDigest.String()) && (request.Method == http.MethodGet || request.Method == http.MethodHead) {
					attestationRequests.Add(1)
					if blocked.Load() {
						writer.Header().Set("Content-Type", "application/json")
						writer.WriteHeader(http.StatusNotFound)
						_, _ = io.WriteString(writer, `{"errors":[{"code":"MANIFEST_UNKNOWN","message":"synthetic unavailable attestation"}]}`)
						return
					}
				}
				handler.ServeHTTP(writer, request)
			}))
			defer source.Close()
			ref, err := name.NewTag(strings.TrimPrefix(source.URL, "http://")+"/fixture/image:latest", name.Insecure)
			archiveRequireNoError(t, err)
			retained := []v1.Image{amd64, arm64}
			var parent []byte
			if scenario.single {
				retained = []v1.Image{amd64}
				archiveRequireNoError(t, remote.Write(ref, amd64, remote.WithContext(ctx)))
				parent, err = amd64.RawManifest()
			} else {
				additions := []mutate.IndexAddendum{
					{Add: amd64, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "amd64"}}},
					{Add: arm64, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: "arm64"}}},
				}
				if scenario.attestation {
					additions = append(additions, mutate.IndexAddendum{Add: attestation, Descriptor: v1.Descriptor{
						Platform:    &v1.Platform{OS: "unknown", Architecture: "unknown"},
						Annotations: map[string]string{"vnd.docker.reference.type": "attestation-manifest"},
					}})
				}
				index := mutate.AppendManifests(empty.Index, additions...)
				archiveRequireNoError(t, remote.WriteIndex(ref, index, remote.WithContext(ctx)))
				parent, err = index.RawManifest()
			}
			archiveRequireNoError(t, err)
			blocked.Store(scenario.inaccessible)
			if scenario.attestation {
				_, err := remote.Get(ref.Context().Digest(attestationDigest.String()), remote.WithContext(ctx))
				if scenario.inaccessible {
					archiveRequireNotFound(t, err)
				} else {
					archiveRequireNoError(t, err)
				}
			}
			attestationRequests.Store(0)
			var omitted []v1.Hash
			if scenario.attestation {
				omitted = append(omitted, attestationDigest)
			}
			if scenario.sparse {
				retained = []v1.Image{amd64}
				arm64Digest, err := arm64.Digest()
				archiveRequireNoError(t, err)
				omitted = append(omitted, arm64Digest)
			}
			parentHash, _, err := v1.SHA256(bytes.NewReader(parent))
			archiveRequireNoError(t, err)
			expected := map[v1.Hash][]byte{parentHash: parent}
			for _, child := range retained {
				for hash, data := range syntheticArchiveContent(t, child) {
					expected[hash] = data
				}
			}
			work := t.TempDir()
			config := filepath.Join(work, "isc.yaml")
			configuration := "apiVersion: mirror.openshift.io/v2alpha1\nkind: ImageSetConfiguration\nmirror:\n  additionalImages:\n    - name: " + ref.String() + "\n"
			if scenario.sparse {
				configuration += "      platforms:\n        - os: linux\n          architecture: amd64\n"
			}
			archiveRequireNoError(t, os.WriteFile(config, []byte(configuration), 0o600))
			cache := filepath.Join(work, "cache")
			output := filepath.Join(work, "output")
			cli := ocmirror.NewRunner(os.Getenv("OC_MIRROR_BINARY"))
			result, err := cli.MirrorToDisk(ctx, config, output, "--cache-dir", cache, "--src-tls-verify=false", "--remove-signatures=true")
			archiveRequireSuccess(t, result, err)
			cacheRoot := filepath.Join(cache, ".oc-mirror", ".cache")
			assertSyntheticArchiveBlobs(t, cacheRoot, expected)
			assertSyntheticArchiveLinks(t, cacheRoot, ref.Context(), retained)
			transport := t.TempDir()
			archives, err := filepath.Glob(filepath.Join(output, "mirror_*.tar"))
			archiveRequireNoError(t, err)
			if len(archives) == 0 {
				t.Fatal("M2D produced no archive")
			}
			for _, archive := range archives {
				data, err := os.ReadFile(archive)
				archiveRequireNoError(t, err)
				archiveRequireNoError(t, os.WriteFile(filepath.Join(transport, filepath.Base(archive)), data, 0o600))
			}
			extracted := t.TempDir()
			extractSyntheticArchives(t, transport, extracted)
			assertSyntheticArchiveBlobs(t, extracted, expected)
			assertSyntheticArchiveLinks(t, extracted, ref.Context(), retained)
			for _, hash := range omitted {
				for _, location := range []string{cacheRoot, extracted} {
					if _, err := readCachedBlob(location, hash); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("excluded manifest %s should be absent from %s: %v", hash, location, err)
					}
				}
			}
			if scenario.attestation {
				if attestationRequests.Load() != 0 {
					t.Fatal("copy fetched an excluded attestation")
				}
			}
			if scenario.name == "available attestation" {
				result, err = cli.MirrorToDisk(ctx, config, output, "--cache-dir", cache, "--src-tls-verify=false", "--remove-signatures=true")
				archiveRequireSuccess(t, result, err)
				incremental := t.TempDir()
				extractSyntheticArchives(t, output, incremental)
				for hash := range expected {
					if _, err := readCachedBlob(incremental, hash); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("unchanged blob %s should not be in incremental archive: %v", hash, err)
					}
				}
				extractSyntheticArchives(t, output, extracted)
				assertSyntheticArchiveBlobs(t, extracted, expected)
			}
			source.Close()
			registryConfig := filepath.Join(t.TempDir(), "registry-config-sparse.yaml")
			archiveRequireNoError(t, os.WriteFile(registryConfig, attestationArchiveRegistryConfig, 0o600))
			target, err := localregistry.Start(ctx, registryConfig, 0)
			archiveRequireNoError(t, err)
			defer func() { archiveRequireNoError(t, target.Stop()) }()
			archiveRequireNoError(t, target.WaitReady(ctx, 10*time.Second))
			targetHost := target.Endpoint()
			result, err = cli.DiskToMirror(ctx, config, transport, targetHost, "--cache-dir", filepath.Join(work, "fresh-d2m-cache"), "--dest-tls-verify=false", "--remove-signatures=true")
			archiveRequireSuccess(t, result, err)
			targetRef, err := name.NewTag(targetHost+"/fixture/image:latest", name.Insecure)
			archiveRequireNoError(t, err)
			copied, err := remote.Get(targetRef, remote.WithContext(ctx))
			archiveRequireNoError(t, err)
			if !bytes.Equal(parent, copied.Manifest) || copied.Digest != parentHash {
				t.Fatal("D2M changed the original parent manifest")
			}
			for _, child := range retained {
				childHash, err := child.Digest()
				archiveRequireNoError(t, err)
				childRef := targetRef.Context().Digest(childHash.String())
				copiedChild, err := remote.Image(childRef, remote.WithContext(ctx))
				archiveRequireNoError(t, err)
				for hash, data := range syntheticArchiveContent(t, copiedChild) {
					if !bytes.Equal(expected[hash], data) {
						t.Fatalf("D2M changed retained content %s", hash)
					}
				}
			}
			for _, hash := range omitted {
				_, err := remote.Get(targetRef.Context().Digest(hash.String()), remote.WithContext(ctx))
				archiveRequireNotFound(t, err)
			}
		})
	}
}

func syntheticArchiveImage(t *testing.T, architecture string) v1.Image {
	t.Helper()
	layer, err := random.Layer(256, types.OCILayer)
	archiveRequireNoError(t, err)
	image, err := mutate.AppendLayers(empty.Image, layer)
	archiveRequireNoError(t, err)
	image = mutate.MediaType(image, types.OCIManifestSchema1)
	image = mutate.ConfigMediaType(image, types.OCIConfigJSON)
	config, err := image.ConfigFile()
	archiveRequireNoError(t, err)
	config.OS = "linux"
	config.Architecture = architecture
	image, err = mutate.ConfigFile(image, config)
	archiveRequireNoError(t, err)
	return image
}

func syntheticArchiveContent(t *testing.T, image v1.Image) map[v1.Hash][]byte {
	t.Helper()
	content := map[v1.Hash][]byte{}
	manifestBytes, err := image.RawManifest()
	archiveRequireNoError(t, err)
	configBytes, err := image.RawConfigFile()
	archiveRequireNoError(t, err)
	for _, data := range [][]byte{manifestBytes, configBytes} {
		hash, _, err := v1.SHA256(bytes.NewReader(data))
		archiveRequireNoError(t, err)
		content[hash] = data
	}
	layers, err := image.Layers()
	archiveRequireNoError(t, err)
	for _, layer := range layers {
		stream, err := layer.Compressed()
		archiveRequireNoError(t, err)
		data, err := io.ReadAll(stream)
		archiveRequireNoError(t, err)
		archiveRequireNoError(t, stream.Close())
		hash, _, err := v1.SHA256(bytes.NewReader(data))
		archiveRequireNoError(t, err)
		content[hash] = data
	}
	return content
}

func assertSyntheticArchiveBlobs(t *testing.T, cache string, expected map[v1.Hash][]byte) {
	t.Helper()
	for hash, data := range expected {
		actual, err := readCachedBlob(cache, hash)
		archiveRequireNoError(t, err)
		if !bytes.Equal(data, actual) {
			t.Fatalf("cached or archived blob %s changed", hash)
		}
	}
}

func assertSyntheticArchiveLinks(t *testing.T, cache string, repository name.Repository, images []v1.Image) {
	t.Helper()
	for _, image := range images {
		hash, err := image.Digest()
		archiveRequireNoError(t, err)
		archiveRequireNoError(t, validateCachedImage(cache, repository.Digest(hash.String())))
	}
}

func extractSyntheticArchives(t *testing.T, source, destination string) {
	t.Helper()
	archives, err := filepath.Glob(filepath.Join(source, "mirror_*.tar"))
	archiveRequireNoError(t, err)
	for _, archive := range archives {
		file, err := os.Open(archive)
		archiveRequireNoError(t, err)
		reader := tar.NewReader(file)
		for {
			header, err := reader.Next()
			if err == io.EOF {
				break
			}
			archiveRequireNoError(t, err)
			if header.Typeflag != tar.TypeReg || !strings.HasPrefix(header.Name, "docker/registry/v2/") {
				continue
			}
			if !filepath.IsLocal(header.Name) {
				t.Fatalf("non-local archive entry %q", header.Name)
			}
			data, err := io.ReadAll(reader)
			archiveRequireNoError(t, err)
			path := filepath.Join(destination, header.Name)
			archiveRequireNoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
			archiveRequireNoError(t, os.WriteFile(path, data, 0o600))
		}
		archiveRequireNoError(t, file.Close())
	}
}

func archiveRequireSuccess(t *testing.T, result *ocmirror.Result, err error) {
	t.Helper()
	archiveRequireNoError(t, err)
	if result.ExitCode != 0 {
		t.Fatalf("oc-mirror exited %d:\n%s\n%s", result.ExitCode, result.Stdout, result.Stderr)
	}
}

func archiveRequireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(fmt.Errorf("synthetic archive fixture: %w", err))
	}
}

func archiveRequireNotFound(t *testing.T, err error) {
	t.Helper()
	var registryError *transport.Error
	if !errors.As(err, &registryError) || registryError.StatusCode != http.StatusNotFound {
		t.Fatalf("expected registry HTTP 404, got %v", err)
	}
}
