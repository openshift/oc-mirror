package release

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	digest "github.com/opencontainers/go-digest"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
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
func TestCollectChannelImagesCrossMajor(t *testing.T) {
	tests := []struct {
		name        string
		graphs      map[string]string
		channels    []v2alpha1.ReleaseChannel
		expVersions []string
		expRequests []string
	}{{
		name: "CandidateChannels",
		graphs: map[string]string{
			"candidate-4.22": testGraph(t, []string{"4.22.19", "4.22.20", "4.22.21", "4.22.23"}, [][2]int{{0, 1}, {1, 2}, {2, 3}}),
			"candidate-4.23": testGraph(t, []string{"4.22.21", "4.23.0-ec.1"}, [][2]int{{0, 1}}),
			"candidate-5.0": testGraph(t, []string{"4.22.19", "4.22.20", "4.22.21", "4.22.23", "5.0.0-rc.1", "5.0.0"},
				[][2]int{{0, 1}, {1, 2}, {1, 5}, {2, 5}, {4, 5}}),
		},
		channels: []v2alpha1.ReleaseChannel{
			{Name: "candidate-4.22", Type: v2alpha1.TypeOCP, MinVersion: "4.22.20", MaxVersion: "4.22.21"},
			{Name: "candidate-5.0", Type: v2alpha1.TypeOCP, MinVersion: "5.0.0", MaxVersion: "5.0.0"},
		},
		expVersions: []string{"4.22.20", "4.22.21", "5.0.0"},
		expRequests: []string{"candidate-4.22", "candidate-5.0", "candidate-5.0"},
	}, {
		name: "StableChannels",
		graphs: map[string]string{
			"stable-4.22": testGraph(t, []string{"4.22.19", "4.22.20"}, [][2]int{{0, 1}}),
			"stable-5.0":  testGraph(t, []string{"4.22.19", "4.22.20", "5.0.0"}, [][2]int{{0, 1}, {1, 2}}),
		},
		channels: []v2alpha1.ReleaseChannel{
			{Name: "stable-4.22", Type: v2alpha1.TypeOCP, MinVersion: "4.22.20", MaxVersion: "4.22.20"},
			{Name: "stable-5.0", Type: v2alpha1.TypeOCP, MinVersion: "5.0.0", MaxVersion: "5.0.0"},
		},
		expVersions: []string{"4.22.20", "5.0.0"},
		expRequests: []string{"stable-4.22", "stable-5.0", "stable-5.0"},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, requestedChannels := newChannelGraphServer(t, tt.graphs)
			t.Setenv("UPDATE_URL_OVERRIDE", endpoint.String())
			o := &CincinnatiSchema{Log: clog.New("trace"), CincinnatiParams: CincinnatiParams{Arch: "amd64", GraphDataDir: t.TempDir()}}
			flagReport := false

			images, errs := o.collectChannelImages(context.Background(), tt.channels, &flagReport)
			assert.Empty(t, errs)

			expImages := make([]string, 0, len(tt.expVersions))
			for _, v := range tt.expVersions {
				expImages = append(expImages, testUpdate(v).Image)
			}
			// Images of the channels and of the cross channel upgrades are not deduplicated here.
			unique := map[string]bool{}
			sources := make([]string, 0, len(images))
			for _, img := range images {
				if !unique[img.Source] {
					unique[img.Source] = true
					sources = append(sources, img.Source)
				}
			}
			assert.ElementsMatch(t, expImages, sources)
			assert.Equal(t, tt.expRequests, requestedChannels())
		})
	}
}

// warnRecorder records the warning messages logged through it.
type warnRecorder struct {
	clog.PluggableLoggerInterface
	warnings []string
}

func (l *warnRecorder) Warn(msg string, _ ...interface{}) {
	l.warnings = append(l.warnings, msg)
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
		// OCPBUGS-136257: the number of minor versions between two major versions is unknown.
		name:    "EUSAcrossMajorVersions",
		channel: v2alpha1.ReleaseChannel{Name: "eus-4.22", MinVersion: "4.22.20", MaxVersion: "5.0.0"},
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
