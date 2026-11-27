package release

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"testing"

	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	digest "github.com/opencontainers/go-digest"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/folder"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type mockSignature struct {
	Log clog.PluggableLoggerInterface
}

func TestGetReleaseReferenceImages(t *testing.T) {
	log := clog.New("trace")

	tmpDir := t.TempDir()
	err := folder.CreateFolders(filepath.Join(tmpDir, "hold-release", "cincinnati-graph-data"))
	assert.NoError(t, err)

	global := &mirror.GlobalOptions{SecurePolicy: false}
	global.WorkingDir = tmpDir

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	opts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Destination:         "oci:test",
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
	}

	cfg := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Platform: v2alpha1.Platform{
					Architectures: []string{"amd64"},
					Graph:         true,
					Channels: []v2alpha1.ReleaseChannel{
						{
							Name: "stable-4.0",
							Full: true,
						},
						{
							Name: "stable-4.1",
						},
						{
							Name:       "stable-4.2",
							MinVersion: "4.2.0",
							MaxVersion: "4.2.10",
						},
						{
							Name:         "stable-4.2",
							ShortestPath: true,
						},
					},
				},
			},
		},
	}

	cfgNoChannels := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Platform: v2alpha1.Platform{
					Architectures: []string{"amd64"},
					Graph:         true,
					Channels: []v2alpha1.ReleaseChannel{
						{
							Type: v2alpha1.TypeOKD,
							Name: "stable-4.0",
						},
					},
				},
			},
		},
	}

	cfgReleaseKubeVirt := v2alpha1.ImageSetConfiguration{
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{
				Platform: v2alpha1.Platform{
					Release:           "test-release-image:v1",
					KubeVirtContainer: true,
				},
			},
		},
	}

	t.Run("TestGetReleaseReferenceImages should pass", func(t *testing.T) {
		c := &mockClient{}
		signature := &mockSignature{Log: log}
		requestQuery := make(chan string, 1)
		defer close(requestQuery)

		handler := getHandlerMulti(t, requestQuery)

		ts := httptest.NewServer(http.HandlerFunc(handler))
		t.Cleanup(ts.Close)

		endpoint, err := url.Parse(ts.URL)
		if err != nil {
			t.Fatalf("should not fail endpoint parse")
		}
		c.url = endpoint
		sch := NewCincinnati(log, nil, &cfg, opts, c, false, signature)
		res, _ := sch.GetReleaseReferenceImages(context.Background())
		if res == nil {
			t.Fatalf("should return a related images")
		}
	})

	t.Run("TestGetReleaseReferenceImages should pass (no channels)", func(t *testing.T) {
		c := &mockClient{}
		signature := &mockSignature{Log: log}
		requestQuery := make(chan string, 1)
		defer close(requestQuery)

		handler := getHandlerMulti(t, requestQuery)

		ts := httptest.NewServer(http.HandlerFunc(handler))
		t.Cleanup(ts.Close)

		endpoint, err := url.Parse(ts.URL)
		if err != nil {
			t.Fatalf("should not fail endpoint parse")
		}
		c.url = endpoint
		sch := NewCincinnati(log, nil, &cfgNoChannels, opts, c, false, signature)
		res, err := sch.GetReleaseReferenceImages(context.Background())

		log.Debug("result from cincinnati %v", res)
		if res == nil || err != nil {
			t.Fatalf("should return a related images")
		}
	})

	t.Run("TestGetReleaseReferenceImages should fail", func(t *testing.T) {
		c := &mockClient{}
		signature := &mockSignature{Log: log}
		requestQuery := make(chan string, 1)
		defer close(requestQuery)

		handler := getHandlerMulti(t, requestQuery)

		ts := httptest.NewServer(http.HandlerFunc(handler))
		t.Cleanup(ts.Close)

		endpoint, err := url.Parse(ts.URL)
		if err != nil {
			t.Fatalf("should not fail endpoint parse")
		}
		c.url = endpoint
		sch := NewCincinnati(log, nil, &cfg, opts, c, true, signature)
		res, _ := sch.GetReleaseReferenceImages(context.Background())

		log.Debug("result from cincinnati %v", res)
		if res == nil {
			t.Fatalf("should return a related images")
		}
	})

	t.Run("TestGetReleaseReferenceImages should pass (platform.release & kubevirt)", func(t *testing.T) {
		c := &mockClient{}
		signature := &mockSignature{Log: log}
		requestQuery := make(chan string, 1)
		defer close(requestQuery)

		handler := getHandlerMulti(t, requestQuery)

		ts := httptest.NewServer(http.HandlerFunc(handler))
		t.Cleanup(ts.Close)

		endpoint, err := url.Parse(ts.URL)
		if err != nil {
			t.Fatalf("should not fail endpoint parse")
		}
		c.url = endpoint

		mm := NewManifest()

		sch := NewCincinnati(log, mm, &cfgReleaseKubeVirt, opts, c, true, signature)
		res, _ := sch.GetReleaseReferenceImages(context.Background())

		log.Debug("result from cincinnati %v", res)
		if res == nil {
			t.Fatalf("should return a related images")
		}
	})
}

type mockManifest struct{}

func NewManifest() mockManifest {
	return mockManifest{}
}

func (o mockSignature) GenerateReleaseSignatures(ctx context.Context, rd []v2alpha1.CopyImageSchema) ([]v2alpha1.CopyImageSchema, error) {
	o.Log.Info("signature verification (mock)")
	return []v2alpha1.CopyImageSchema{}, nil
}

func (o mockManifest) ImageDigest(ctx context.Context, srcContext *types.SystemContext, img string) (string, error) {
	return "123456546546546546546546546", nil
}

func (o mockManifest) GetOCIImageIndex(dir string) (*specv1.Index, error) {
	return nil, nil
}

func (o mockManifest) GetOCIImageManifest(file string) (*specv1.Manifest, error) {
	return nil, nil
}

func (o mockManifest) GetOCIImageFromIndex(dir string) (gcrv1.Image, error) { //nolint:ireturn // as expected by go-containerregistry
	return nil, nil
}

func (o mockManifest) GetOperatorConfig(file string) (*v2alpha1.OperatorConfigSchema, error) {
	return &v2alpha1.OperatorConfigSchema{}, nil
}

func (o mockManifest) ExtractOCILayers(_ gcrv1.Image, toPath, label string) error {
	return nil
}

func (o mockManifest) GetReleaseSchema(filePath string) ([]v2alpha1.RelatedImage, error) {
	return []v2alpha1.RelatedImage{}, nil
}

func (o mockManifest) ConvertOCIIndexToSingleManifest(dir string, oci *specv1.Index) error {
	return nil
}

func (o mockManifest) ImageManifest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string, instanceDigest *digest.Digest) ([]byte, string, error) {
	return nil, "", nil
}

func (o mockManifest) GetManifestListDigests(ctx context.Context, sourceCtx *types.SystemContext, source string) ([]string, error) {
	return nil, nil
}

// OCPBUGS-136257: release channels of two major versions in the same image set configuration.
func TestCollectChannelImagesCrossMajorTargetFirst(t *testing.T) {
	endpoint, requestedChannels := newChannelGraphServer(t, map[string]string{
		"stable-4.22": testGraph(t, []string{"4.22.1"}, nil),
		"stable-5.0":  testGraph(t, []string{"4.22.1", "4.22.5", "5.0.0"}, [][2]int{{0, 1}, {1, 2}}),
	})
	t.Setenv("UPDATE_URL_OVERRIDE", endpoint.String())
	o := &CincinnatiSchema{Log: clog.New("error"), CincinnatiParams: CincinnatiParams{Arch: "amd64", GraphDataDir: t.TempDir()}}
	channels := []v2alpha1.ReleaseChannel{
		{Name: "stable-4.22", Type: v2alpha1.TypeOCP, MinVersion: "4.22.1", MaxVersion: "4.22.1"},
		{Name: "stable-5.0", Type: v2alpha1.TypeOCP, MinVersion: "5.0.0", MaxVersion: "5.0.0"},
	}
	warned := false

	images, errs := o.collectChannelImages(context.Background(), channels, &warned)
	require.Empty(t, errs)
	assert.ElementsMatch(t, releaseImages("4.22.1", "4.22.5", "5.0.0"), imageSources(images))
	// The first two requests are for per-channel collection. Only the target is needed after that.
	assert.Equal(t, []string{"stable-4.22", "stable-5.0", "stable-5.0"}, requestedChannels())
}

// warnRecorder records the warning messages logged through it.
type warnRecorder struct {
	clog.PluggableLoggerInterface
	warnings []string
}

func (l *warnRecorder) Warn(msg string, args ...interface{}) {
	l.warnings = append(l.warnings, fmt.Sprintf(msg, args...))
}

func TestWarnEUSChannel(t *testing.T) {
	tests := []struct {
		name            string
		channel         v2alpha1.ReleaseChannel
		alreadyReported bool
		expReported     bool
	}{{
		name:        "EUSTwoMinorVersions",
		channel:     v2alpha1.ReleaseChannel{Name: "eus-4.22", MinVersion: "4.20.0", MaxVersion: "4.22.0"},
		expReported: true,
	}, {
		name:    "EUSOneMinorVersion",
		channel: v2alpha1.ReleaseChannel{Name: "eus-4.22", MinVersion: "4.21.0", MaxVersion: "4.22.5"},
	}, {
		name:    "EUSPatchVersions",
		channel: v2alpha1.ReleaseChannel{Name: "eus-4.22", MinVersion: "4.22.0", MaxVersion: "4.22.9"},
	}, {
		// OCPBUGS-136257: ranges that reach a later major version warn too.
		name:        "EUSAcrossMajorVersions",
		channel:     v2alpha1.ReleaseChannel{Name: "eus-4.22", MinVersion: "4.22.20", MaxVersion: "5.0.0"},
		expReported: true,
	}, {
		name:        "EUSAcrossMajorAndMinorVersions",
		channel:     v2alpha1.ReleaseChannel{Name: "eus-4.22", MinVersion: "4.22.0", MaxVersion: "5.2.0"},
		expReported: true,
	}, {
		name:    "EUSReversedAcrossMajorVersions",
		channel: v2alpha1.ReleaseChannel{Name: "eus-5.0", MinVersion: "5.0.0", MaxVersion: "4.22.0"},
	}, {
		// The minor version distance does not overflow.
		name:    "EUSLargestMinorVersions",
		channel: v2alpha1.ReleaseChannel{Name: "eus-4.22", MinVersion: "4.18446744073709551614.0", MaxVersion: "4.18446744073709551615.0"},
	}, {
		name:    "NonEUSAcrossMajorVersions",
		channel: v2alpha1.ReleaseChannel{Name: "stable-5.0", MinVersion: "4.22.20", MaxVersion: "5.0.0"},
	}, {
		name:    "EUSReversedRange",
		channel: v2alpha1.ReleaseChannel{Name: "eus-4.22", MinVersion: "4.22.0", MaxVersion: "4.20.0"},
	}, {
		name:            "AlreadyReported",
		channel:         v2alpha1.ReleaseChannel{Name: "eus-4.22", MinVersion: "4.20.0", MaxVersion: "4.22.0"},
		alreadyReported: true,
		expReported:     true,
	}, {
		name:    "NoMinVersion",
		channel: v2alpha1.ReleaseChannel{Name: "eus-4.22", MaxVersion: "4.22.0"},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := &warnRecorder{PluggableLoggerInterface: clog.New("trace")}
			o := &CincinnatiSchema{Log: log}

			reported := o.warnEUSChannel(tt.channel, tt.alreadyReported)
			assert.Equal(t, tt.expReported, reported)
			if tt.expReported && !tt.alreadyReported {
				assert.Len(t, log.warnings, 1)
			} else {
				assert.Empty(t, log.warnings)
			}
		})
	}
}

// passthroughSignature returns the release images unchanged, as if their signatures were valid.
type passthroughSignature struct{}

func (passthroughSignature) GenerateReleaseSignatures(_ context.Context, images []v2alpha1.CopyImageSchema) ([]v2alpha1.CopyImageSchema, error) {
	return images, nil
}

// imageSources returns the unique sources of the images.
func imageSources(images []v2alpha1.CopyImageSchema) []string {
	seen := map[string]bool{}
	sources := []string{}
	for _, img := range images {
		if !seen[img.Source] {
			seen[img.Source] = true
			sources = append(sources, img.Source)
		}
	}
	return sources
}

// releaseImages returns the release images of the versions.
func releaseImages(versions ...string) []string {
	images := make([]string, 0, len(versions))
	for _, v := range versions {
		images = append(images, testUpdate(v).Image)
	}
	return images
}

// A third configured channel is not used as a bridge.
func TestGetCrossChannelDownloadsDoesNotSearchOtherConfiguredChannels(t *testing.T) {
	ocpEndpoint, ocpRequests := newChannelGraphServer(t, map[string]string{
		"stable-4.22":   testGraph(t, []string{"4.22.1"}, nil),
		"stable-5.0":    testGraph(t, []string{"5.0.0", "5.0.1"}, [][2]int{{0, 1}}),
		"candidate-5.0": testGraph(t, []string{"4.22.1", "5.0.0"}, [][2]int{{0, 1}}),
	})
	t.Setenv("UPDATE_URL_OVERRIDE", ocpEndpoint.String())
	log := &warnRecorder{PluggableLoggerInterface: clog.New("error")}
	cs := CincinnatiSchema{
		Log:              log,
		CincinnatiParams: CincinnatiParams{Arch: "amd64", GraphDataDir: t.TempDir()},
	}
	require.NoError(t, cs.NewOCPClient())
	channels := []v2alpha1.ReleaseChannel{
		{Name: "stable-4.22", Type: v2alpha1.TypeOCP, MinVersion: "4.22.1", MaxVersion: "4.22.1"},
		{Name: "stable-5.0", Type: v2alpha1.TypeOCP, MinVersion: "5.0.1", MaxVersion: "5.0.1"},
		{Name: "candidate-5.0", Type: v2alpha1.TypeOCP, MinVersion: "5.0.0", MaxVersion: "5.0.0"},
	}

	images, err := getCrossChannelDownloads(context.Background(), cs, channels)
	require.NoError(t, err)
	assert.ElementsMatch(t, releaseImages("4.22.1", "5.0.1"), imageSources(images))
	require.Len(t, log.warnings, 1)
	assert.Contains(t, log.warnings[0], "the two graphs publish no unconditional update path")
	assert.Equal(t, []string{"stable-5.0", "stable-4.22"}, ocpRequests())
}

// When the selected source and target channels have no path to a later major version, the other
// configured OCP channels with the same start or target version are tried in a fixed order.
func TestGetCrossChannelDownloadsTiedChannels(t *testing.T) {
	ocp := func(name, version string) v2alpha1.ReleaseChannel {
		return v2alpha1.ReleaseChannel{Name: name, Type: v2alpha1.TypeOCP, MinVersion: version, MaxVersion: version}
	}
	noPath5 := testGraph(t, []string{"5.0.0", "5.0.1"}, [][2]int{{0, 1}})
	source := testGraph(t, []string{"4.22.1"}, nil)
	tests := []struct {
		name      string
		graphs    map[string]string
		failing   []string
		stored    map[string]string
		mode      string
		canceled  bool
		channels  []v2alpha1.ReleaseChannel
		expImages []string
		expReqs   []string
		expWarn   string
		expErr    string
	}{
		{
			name: "SelectedTargetPath",
			graphs: map[string]string{
				"stable-4.22": source,
				"stable-5.0":  testGraph(t, []string{"4.22.1", "4.22.3", "5.0.1"}, [][2]int{{0, 1}, {1, 2}}),
				"fast-5.0":    testGraph(t, []string{"4.22.1", "4.22.5", "5.0.1"}, [][2]int{{0, 1}, {1, 2}}),
			},
			channels:  []v2alpha1.ReleaseChannel{ocp("stable-4.22", "4.22.1"), ocp("fast-5.0", "5.0.1"), ocp("stable-5.0", "5.0.1")},
			expImages: releaseImages("4.22.1", "4.22.3", "5.0.1"),
			expReqs:   []string{"stable-5.0"},
		},
		{
			name: "TiedTarget",
			graphs: map[string]string{
				"stable-4.22": source,
				"stable-5.0":  noPath5,
				"fast-5.0":    testGraph(t, []string{"4.22.1", "4.22.5", "5.0.1"}, [][2]int{{0, 1}, {1, 2}}),
			},
			channels:  []v2alpha1.ReleaseChannel{ocp("stable-4.22", "4.22.1"), ocp("fast-5.0", "5.0.1"), ocp("stable-5.0", "5.0.1")},
			expImages: releaseImages("4.22.1", "4.22.5", "5.0.1"),
			expReqs:   []string{"stable-5.0", "stable-4.22", "fast-5.0"},
		},
		{
			name: "TiedSource",
			graphs: map[string]string{
				"stable-4.22": source,
				"fast-4.22":   testGraph(t, []string{"4.22.1", "4.22.5", "5.0.0"}, [][2]int{{0, 1}, {1, 2}}),
				"stable-5.0":  noPath5,
			},
			channels:  []v2alpha1.ReleaseChannel{ocp("fast-4.22", "4.22.1"), ocp("stable-4.22", "4.22.1"), ocp("stable-5.0", "5.0.1")},
			expImages: releaseImages("4.22.1", "4.22.5", "5.0.0", "5.0.1"),
			expReqs:   []string{"stable-5.0", "stable-4.22", "fast-4.22"},
		},
		{
			name: "FirstTiedChannelInOrder",
			graphs: map[string]string{
				"stable-4.22":   source,
				"stable-5.0":    noPath5,
				"fast-5.0":      testGraph(t, []string{"4.22.1", "4.22.5", "5.0.1"}, [][2]int{{0, 1}, {1, 2}}),
				"candidate-5.0": testGraph(t, []string{"4.22.1", "4.22.3", "5.0.1"}, [][2]int{{0, 1}, {1, 2}}),
			},
			channels:  []v2alpha1.ReleaseChannel{ocp("stable-5.0", "5.0.1"), ocp("candidate-5.0", "5.0.1"), ocp("stable-4.22", "4.22.1"), ocp("fast-5.0", "5.0.1")},
			expImages: releaseImages("4.22.1", "4.22.5", "5.0.1"),
			expReqs:   []string{"stable-5.0", "stable-4.22", "fast-5.0"},
		},
		{
			name: "FirstTiedChannelInOrderReversedConfig",
			graphs: map[string]string{
				"stable-4.22":   source,
				"stable-5.0":    noPath5,
				"fast-5.0":      testGraph(t, []string{"4.22.1", "4.22.5", "5.0.1"}, [][2]int{{0, 1}, {1, 2}}),
				"candidate-5.0": testGraph(t, []string{"4.22.1", "4.22.3", "5.0.1"}, [][2]int{{0, 1}, {1, 2}}),
			},
			channels:  []v2alpha1.ReleaseChannel{ocp("fast-5.0", "5.0.1"), ocp("stable-4.22", "4.22.1"), ocp("candidate-5.0", "5.0.1"), ocp("stable-5.0", "5.0.1")},
			expImages: releaseImages("4.22.1", "4.22.5", "5.0.1"),
			expReqs:   []string{"stable-5.0", "stable-4.22", "fast-5.0"},
		},
		{
			name: "BothTiedChannels",
			graphs: map[string]string{
				"stable-4.22": source,
				"fast-4.22":   testGraph(t, []string{"4.22.1", "4.22.5"}, [][2]int{{0, 1}}),
				"stable-5.0":  noPath5,
				"fast-5.0":    testGraph(t, []string{"4.22.5", "5.0.1"}, [][2]int{{0, 1}}),
			},
			channels:  []v2alpha1.ReleaseChannel{ocp("fast-4.22", "4.22.1"), ocp("stable-4.22", "4.22.1"), ocp("fast-5.0", "5.0.1"), ocp("stable-5.0", "5.0.1")},
			expImages: releaseImages("4.22.1", "4.22.5", "5.0.1"),
			expReqs:   []string{"stable-5.0", "stable-4.22", "fast-4.22", "fast-5.0"},
		},
		{
			name: "NoTiedChannelPath",
			graphs: map[string]string{
				"stable-4.22": source,
				"stable-5.0":  noPath5,
				"fast-5.0":    noPath5,
			},
			channels:  []v2alpha1.ReleaseChannel{ocp("stable-4.22", "4.22.1"), ocp("fast-5.0", "5.0.1"), ocp("stable-5.0", "5.0.1")},
			expImages: releaseImages("4.22.1", "5.0.1"),
			expReqs:   []string{"stable-5.0", "stable-4.22", "fast-5.0"},
			expWarn:   fmt.Sprintf("the graphs of the channels %q, %q, %q publish no unconditional update path through generally available releases", "fast-5.0", "stable-4.22", "stable-5.0"),
		},
		{
			name: "UntiedUnconfiguredAndOKDChannelsNotSearched",
			graphs: map[string]string{
				"stable-4.22":   source,
				"stable-5.0":    noPath5,
				"candidate-5.0": testGraph(t, []string{"4.22.1", "5.0.0", "5.0.1"}, [][2]int{{0, 1}, {1, 2}}),
				"fast-5.0":      testGraph(t, []string{"4.22.1", "5.0.1"}, [][2]int{{0, 1}}),
				"eus-5.0":       testGraph(t, []string{"4.22.1", "5.0.1"}, [][2]int{{0, 1}}),
			},
			channels: []v2alpha1.ReleaseChannel{
				ocp("stable-4.22", "4.22.1"), ocp("stable-5.0", "5.0.1"), ocp("candidate-5.0", "5.0.0"),
				{Name: "fast-5.0", Type: v2alpha1.TypeOKD, MinVersion: "5.0.1", MaxVersion: "5.0.1"},
			},
			expImages: releaseImages("4.22.1", "5.0.1"),
			expReqs:   []string{"stable-5.0", "stable-4.22"},
			expWarn:   "the two graphs publish no unconditional update path through generally available releases",
		},
		{
			name: "TiedTargetWithoutRequestedVersionSkipped",
			graphs: map[string]string{
				"stable-4.22": source,
				"stable-5.0":  noPath5,
				"fast-5.0":    testGraph(t, []string{"4.22.1", "5.0.0"}, [][2]int{{0, 1}}),
			},
			channels:  []v2alpha1.ReleaseChannel{ocp("stable-4.22", "4.22.1"), ocp("fast-5.0", "5.0.1"), ocp("stable-5.0", "5.0.1")},
			expImages: releaseImages("4.22.1", "5.0.1"),
			expReqs:   []string{"stable-5.0", "stable-4.22", "fast-5.0"},
			expWarn:   fmt.Sprintf("the graphs of the channels %q, %q, %q publish no unconditional update path", "fast-5.0", "stable-4.22", "stable-5.0"),
		},
		{
			name: "TiedChannelCanceledWithSavedGraphs",
			stored: map[string]string{
				"stable-4.22": source,
				"stable-5.0":  noPath5,
				"fast-5.0":    testGraph(t, []string{"4.22.1", "4.22.5", "5.0.1"}, [][2]int{{0, 1}, {1, 2}}),
			},
			mode:     mirror.DiskToMirror,
			canceled: true,
			channels: []v2alpha1.ReleaseChannel{ocp("stable-4.22", "4.22.1"), ocp("fast-5.0", "5.0.1"), ocp("stable-5.0", "5.0.1")},
			expErr:   "failed to get upgrade graph: searching the tied channel graphs: context canceled",
		},
		{
			name: "TiedChannelInvalidSavedGraph",
			graphs: map[string]string{
				"stable-4.22": source,
				"stable-5.0":  noPath5,
			},
			stored:   map[string]string{"fast-5.0": "not a graph"},
			channels: []v2alpha1.ReleaseChannel{ocp("stable-4.22", "4.22.1"), ocp("fast-5.0", "5.0.1"), ocp("stable-5.0", "5.0.1")},
			expReqs:  []string{"stable-5.0", "stable-4.22"},
			expErr:   `failed to get upgrade graph: APIRequestError: channel "fast-5.0"`,
		},
		{
			name: "TiedChannelGraphError",
			graphs: map[string]string{
				"stable-4.22": source,
				"stable-5.0":  noPath5,
			},
			failing:  []string{"fast-5.0"},
			channels: []v2alpha1.ReleaseChannel{ocp("stable-4.22", "4.22.1"), ocp("fast-5.0", "5.0.1"), ocp("stable-5.0", "5.0.1")},
			expReqs:  []string{"stable-5.0", "stable-4.22", "fast-5.0"},
			expErr:   `failed to get upgrade graph: APIRequestError: channel "fast-5.0"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, requestedChannels := newChannelGraphServer(t, tt.graphs, tt.failing...)
			t.Setenv("UPDATE_URL_OVERRIDE", endpoint.String())
			log := &warnRecorder{PluggableLoggerInterface: clog.New("error")}
			graphDataDir := t.TempDir()
			for channel, graph := range tt.stored {
				require.NoError(t, os.WriteFile(filepath.Join(graphDataDir, "amd64-"+channel+".json"), []byte(graph), 0o600))
			}
			cs := CincinnatiSchema{Log: log, CincinnatiParams: CincinnatiParams{Arch: "amd64", GraphDataDir: graphDataDir}}
			cs.Opts.Mode = tt.mode
			require.NoError(t, cs.NewOCPClient())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.canceled {
				cancel()
			}

			images, err := getCrossChannelDownloads(ctx, cs, tt.channels)
			assert.Equal(t, tt.expReqs, requestedChannels())
			if tt.expErr != "" {
				require.ErrorContains(t, err, tt.expErr)
				assert.Empty(t, log.warnings)
				return
			}
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expImages, imageSources(images))
			if tt.expWarn == "" {
				assert.Empty(t, log.warnings)
				return
			}
			require.Len(t, log.warnings, 1)
			assert.Contains(t, log.warnings[0], tt.expWarn)
		})
	}
}

// Disk to mirror replays the tied channel choice from the graphs saved by mirror to disk, including
// the channel bounds resolved from them, without the Cincinnati API.
func TestGetReleaseReferenceImagesTiedChannelDiskToMirror(t *testing.T) {
	endpoint, requestedChannels := newChannelGraphServer(t, map[string]string{
		"stable-4.22": testGraph(t, []string{"4.22.1"}, nil),
		"stable-5.0":  testGraph(t, []string{"5.0.0", "5.0.1"}, [][2]int{{0, 1}}),
		"fast-5.0":    testGraph(t, []string{"4.22.1", "4.22.5", "5.0.1"}, [][2]int{{0, 1}, {1, 2}}),
	})
	unreachable := httptest.NewServer(http.NotFoundHandler())
	unreachableURL := unreachable.URL
	unreachable.Close()
	workingDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workingDir, releaseImageExtractDir, cincinnatiGraphDataDir), 0o755))
	newConfig := func() *v2alpha1.ImageSetConfiguration {
		return &v2alpha1.ImageSetConfiguration{ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{Platform: v2alpha1.Platform{Channels: []v2alpha1.ReleaseChannel{
				{Name: "stable-4.22", Type: v2alpha1.TypeOCP, MinVersion: "4.22.1", MaxVersion: "4.22.1"},
				{Name: "fast-5.0", Type: v2alpha1.TypeOCP},
				{Name: "stable-5.0", Type: v2alpha1.TypeOCP},
			}}},
		}}
	}
	run := func(mode string, url string) ([]v2alpha1.CopyImageSchema, *warnRecorder) {
		t.Setenv("UPDATE_URL_OVERRIDE", url)
		log := &warnRecorder{PluggableLoggerInterface: clog.New("error")}
		opts := mirror.CopyOptions{Global: &mirror.GlobalOptions{WorkingDir: workingDir}, Mode: mode}
		images, err := NewCincinnati(log, nil, newConfig(), opts, &mockClient{}, false, passthroughSignature{}).GetReleaseReferenceImages(context.Background())
		require.NoError(t, err)
		return images, log
	}

	m2d, log := run(mirror.MirrorToDisk, endpoint.String())
	assert.ElementsMatch(t, releaseImages("4.22.1", "4.22.5", "5.0.1"), slices.Compact(sortedSources(m2d)))
	assert.Empty(t, log.warnings)
	requests := len(requestedChannels())

	d2m, log := run(mirror.DiskToMirror, unreachableURL)
	assert.Equal(t, sortedSources(m2d), sortedSources(d2m))
	assert.Empty(t, log.warnings)
	assert.Len(t, requestedChannels(), requests, "disk to mirror must not query the Cincinnati API")
}

// The graphs that the Cincinnati API returns for a version may differ from the graphs used by the
// per-channel collection. Disk to mirror must still replay the same releases: a tied channel reuses
// the graph saved by its per-channel collection instead of replacing it.
func TestGetReleaseReferenceImagesTiedChannelVersionedGraphs(t *testing.T) {
	unversioned := map[string]string{
		"stable-4.22": testGraph(t, []string{"4.22.1"}, nil),
		"stable-5.0":  testGraph(t, []string{"5.0.0", "5.0.1"}, [][2]int{{0, 1}}),
		"fast-5.0":    testGraph(t, []string{"4.22.1", "4.22.5", "5.0.1"}, [][2]int{{0, 1}, {1, 2}}),
	}
	// A request for a version also returns a later fast-5.0 release.
	versioned := map[string]string{
		"fast-5.0": testGraph(t, []string{"4.22.1", "4.22.5", "5.0.1", "5.0.2"}, [][2]int{{0, 1}, {1, 2}, {2, 3}}),
	}
	var mu sync.Mutex
	var requests []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		channel, version := r.URL.Query().Get("channel"), r.URL.Query().Get("version")
		mu.Lock()
		requests = append(requests, channel+"@"+version)
		mu.Unlock()
		body, ok := versioned[channel]
		if !ok || version == "" {
			body = unversioned[channel]
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()
	unreachable := httptest.NewServer(http.NotFoundHandler())
	unreachableURL := unreachable.URL
	unreachable.Close()
	workingDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workingDir, releaseImageExtractDir, cincinnatiGraphDataDir), 0o755))
	run := func(mode string, url string) ([]string, []string) {
		t.Setenv("UPDATE_URL_OVERRIDE", url)
		log := &warnRecorder{PluggableLoggerInterface: clog.New("error")}
		cfg := &v2alpha1.ImageSetConfiguration{ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			Mirror: v2alpha1.Mirror{Platform: v2alpha1.Platform{Channels: []v2alpha1.ReleaseChannel{
				{Name: "stable-4.22", Type: v2alpha1.TypeOCP, MinVersion: "4.22.1", MaxVersion: "4.22.1"},
				{Name: "fast-5.0", Type: v2alpha1.TypeOCP},
				{Name: "stable-5.0", Type: v2alpha1.TypeOCP},
			}}},
		}}
		opts := mirror.CopyOptions{Global: &mirror.GlobalOptions{WorkingDir: workingDir}, Mode: mode}
		images, err := NewCincinnati(log, nil, cfg, opts, &mockClient{}, false, passthroughSignature{}).GetReleaseReferenceImages(context.Background())
		require.NoError(t, err)
		return slices.Compact(sortedSources(images)), log.warnings
	}

	m2d, warnings := run(mirror.MirrorToDisk, ts.URL)
	assert.Equal(t, releaseImages("4.22.1", "4.22.5", "5.0.1"), m2d)
	assert.Empty(t, warnings)
	mu.Lock()
	m2dRequests := slices.Clone(requests)
	mu.Unlock()
	assert.NotContains(t, m2dRequests, "fast-5.0@4.22.1", "the tied channel reuses its saved per-channel graph")

	d2m, warnings := run(mirror.DiskToMirror, unreachableURL)
	assert.Equal(t, m2d, d2m)
	assert.Empty(t, warnings)
	mu.Lock()
	assert.Equal(t, m2dRequests, requests, "disk to mirror must not query the Cincinnati API")
	mu.Unlock()
}

// sortedSources returns the sorted image sources.
func sortedSources(images []v2alpha1.CopyImageSchema) []string {
	sources := imageSources(images)
	sort.Strings(sources)
	return sources
}

// newArchChannelGraphServer mocks a Cincinnati API serving the graphs by "<arch>/<channel>". It
// returns the server URL and a function listing the requested "<arch>/<channel>" in order.
func newArchChannelGraphServer(t *testing.T, graphs map[string]string) (*url.URL, func() []string) {
	t.Helper()
	emptyGraph := testGraph(t, nil, nil)
	var mu sync.Mutex
	var requested []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("arch") + "/" + r.URL.Query().Get("channel")
		mu.Lock()
		requested = append(requested, key)
		mu.Unlock()
		body, ok := graphs[key]
		if !ok {
			body = emptyGraph
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(ts.Close)
	endpoint, err := url.Parse(ts.URL)
	require.NoError(t, err)
	return endpoint, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), requested...)
	}
}

// Each architecture uses the graphs of its own channels.
func TestGetReleaseReferenceImagesConnectingChannelArchitectures(t *testing.T) {
	endpoint, requested := newArchChannelGraphServer(t, map[string]string{
		"amd64/stable-4.22": testGraph(t, []string{"4.22.1", "4.22.5"}, [][2]int{{0, 1}}),
		"amd64/stable-5.1":  testGraph(t, []string{"4.22.5", "5.0.0", "5.1.0"}, [][2]int{{0, 1}, {1, 2}}),
		"arm64/stable-4.22": testGraph(t, []string{"4.22.1", "4.22.7"}, [][2]int{{0, 1}}),
		"arm64/stable-5.1":  testGraph(t, []string{"4.22.7", "5.0.0", "5.1.0"}, [][2]int{{0, 1}, {1, 2}}),
	})
	t.Setenv("UPDATE_URL_OVERRIDE", endpoint.String())
	workingDir := t.TempDir()
	graphDataDir := filepath.Join(workingDir, releaseImageExtractDir, cincinnatiGraphDataDir)
	require.NoError(t, os.MkdirAll(graphDataDir, 0o755))
	cfg := &v2alpha1.ImageSetConfiguration{ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
		Mirror: v2alpha1.Mirror{Platform: v2alpha1.Platform{
			//nolint:staticcheck // SA1019: the deprecated Architectures field is the per architecture collection under test.
			Architectures: []string{"amd64", "arm64"},
			Channels: []v2alpha1.ReleaseChannel{
				{Name: "stable-4.22", Type: v2alpha1.TypeOCP, MinVersion: "4.22.1", MaxVersion: "4.22.1"},
				{Name: "stable-5.1", Type: v2alpha1.TypeOCP, MinVersion: "5.1.0", MaxVersion: "5.1.0"},
			},
		}},
	}}
	opts := mirror.CopyOptions{Global: &mirror.GlobalOptions{WorkingDir: workingDir}, Mode: mirror.MirrorToDisk}
	sch := NewCincinnati(clog.New("error"), nil, cfg, opts, &mockClient{}, false, passthroughSignature{})

	images, err := sch.GetReleaseReferenceImages(context.Background())
	require.NoError(t, err)
	assert.ElementsMatch(t, releaseImages("4.22.1", "4.22.5", "4.22.7", "5.0.0", "5.1.0"), imageSources(images))
	for _, arch := range []string{"amd64", "arm64"} {
		for _, ch := range []string{"stable-4.22", "stable-5.1"} {
			assert.FileExists(t, filepath.Join(graphDataDir, arch+"-"+ch+".json"))
			assert.Contains(t, requested(), arch+"/"+ch)
		}
	}
}

// Disk to mirror replays the two saved graphs and requires the source graph when the target alone has no path.
func TestGetCrossChannelDownloadsConnectingChannelsDiskToMirror(t *testing.T) {
	endpoint, requestedChannels := newChannelGraphServer(t, map[string]string{
		"stable-4.22": testGraph(t, []string{"4.22.1", "4.22.5"}, [][2]int{{0, 1}}),
		"stable-5.1":  testGraph(t, []string{"4.22.5", "5.0.0", "5.1.0"}, [][2]int{{0, 1}, {1, 2}}),
	})
	unreachable := httptest.NewServer(http.NotFoundHandler())
	unreachableURL := unreachable.URL
	unreachable.Close()

	channels := []v2alpha1.ReleaseChannel{
		{Name: "stable-5.1", Type: v2alpha1.TypeOCP, MinVersion: "5.1.0", MaxVersion: "5.1.0"},
		{Name: "stable-4.22", Type: v2alpha1.TypeOCP, MinVersion: "4.22.1", MaxVersion: "4.22.1"},
	}
	graphDataDir := t.TempDir()
	cs := CincinnatiSchema{Log: clog.New("error"), CincinnatiParams: CincinnatiParams{Arch: "amd64", GraphDataDir: graphDataDir}}
	cs.Opts.Mode = mirror.MirrorToDisk
	t.Setenv("UPDATE_URL_OVERRIDE", endpoint.String())
	require.NoError(t, cs.NewOCPClient())
	m2d, err := getCrossChannelDownloads(context.Background(), cs, channels)
	require.NoError(t, err)
	assert.ElementsMatch(t, releaseImages("4.22.1", "4.22.5", "5.0.0", "5.1.0"), imageSources(m2d))
	requests := len(requestedChannels())

	files, err := os.ReadDir(graphDataDir)
	require.NoError(t, err)
	stored := make([]string, 0, len(files))
	for _, f := range files {
		stored = append(stored, f.Name())
	}
	assert.ElementsMatch(t, []string{"amd64-stable-5.1.json", "amd64-stable-4.22.json"}, stored)

	d2m := cs
	d2m.Opts.Mode = mirror.DiskToMirror
	t.Setenv("UPDATE_URL_OVERRIDE", unreachableURL)
	require.NoError(t, d2m.NewOCPClient())
	replayed, err := getCrossChannelDownloads(context.Background(), d2m, channels)
	require.NoError(t, err)
	assert.ElementsMatch(t, imageSources(m2d), imageSources(replayed))
	assert.Len(t, requestedChannels(), requests, "disk to mirror must not query the Cincinnati API")

	require.NoError(t, os.Remove(filepath.Join(graphDataDir, "amd64-stable-4.22.json")))
	_, err = getCrossChannelDownloads(context.Background(), d2m, channels)
	require.EqualError(t, err, `failed to get upgrade graph: APIRequestError: channel "stable-4.22": NoGraphData: No graph data found on disk`)
}

// The mirror run continues with the known releases when there is no cross major version path.
func TestGetReleaseReferenceImagesNoUpgradePathContinues(t *testing.T) {
	endpoint, _ := newChannelGraphServer(t, map[string]string{
		"stable-4.22": testGraph(t, []string{"4.22.1"}, nil),
		"stable-5.0":  conditionalEdgesOnlyGraph,
	})
	t.Setenv("UPDATE_URL_OVERRIDE", endpoint.String())
	workingDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workingDir, releaseImageExtractDir, cincinnatiGraphDataDir), 0o755))
	cfg := &v2alpha1.ImageSetConfiguration{ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
		Mirror: v2alpha1.Mirror{Platform: v2alpha1.Platform{Channels: []v2alpha1.ReleaseChannel{
			{Name: "stable-4.22", Type: v2alpha1.TypeOCP, MinVersion: "4.22.1", MaxVersion: "4.22.1"},
			{Name: "stable-5.0", Type: v2alpha1.TypeOCP},
		}}},
	}}
	opts := mirror.CopyOptions{Global: &mirror.GlobalOptions{WorkingDir: workingDir}, Mode: mirror.MirrorToDisk}
	log := &warnRecorder{PluggableLoggerInterface: clog.New("error")}
	sch := NewCincinnati(log, nil, cfg, opts, &mockClient{}, false, passthroughSignature{})

	images, err := sch.GetReleaseReferenceImages(context.Background())
	require.NoError(t, err)
	assert.ElementsMatch(t, releaseImages("4.22.1", "5.0.0"), imageSources(images))
	require.Len(t, log.warnings, 1)
	assert.Contains(t, log.warnings[0], "NoUpgradePath: no qualifying unconditional upgrade path from version 4.22.1")
	assert.Contains(t, log.warnings[0], "conditional update edges are not followed")
}
