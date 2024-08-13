package release

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	_ "k8s.io/klog/v2" // integration tests set glog flags.
)

type mockSignature struct {
	Log clog.PluggableLoggerInterface
}

func TestGetReleaseReferenceImages(t *testing.T) {

	log := clog.New("trace")

	global := &mirror.GlobalOptions{SecurePolicy: false}

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
		sch := NewCincinnati(log, &cfg, opts, c, false, signature)
		res := sch.GetReleaseReferenceImages(context.Background())

		log.Debug("result from cincinnati %v", res)
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
		sch := NewCincinnati(log, &cfgNoChannels, opts, c, false, signature)
		res := sch.GetReleaseReferenceImages(context.Background())

		log.Debug("result from cincinnati %v", res)
		if res == nil {
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
		sch := NewCincinnati(log, &cfg, opts, c, true, signature)
		res := sch.GetReleaseReferenceImages(context.Background())

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
		sch := NewCincinnati(log, &cfgReleaseKubeVirt, opts, c, true, signature)
		res := sch.GetReleaseReferenceImages(context.Background())

		log.Debug("result from cincinnati %v", res)
		if res == nil {
			t.Fatalf("should return a related images")
		}
	})

}

func (o mockSignature) GenerateReleaseSignatures(ctx context.Context, rd []v2alpha1.CopyImageSchema) ([]v2alpha1.CopyImageSchema, error) {
	o.Log.Info("signature verification (mock)")
	return []v2alpha1.CopyImageSchema{}, nil
}
