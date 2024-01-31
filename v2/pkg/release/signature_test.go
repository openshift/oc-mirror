package release

import (
	"context"
	"os"
	"testing"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
	"github.com/stretchr/testify/assert"
)

func TestReleaseSignature(t *testing.T) {
	log := clog.New("trace")

	tempDir := t.TempDir()
	_ = os.MkdirAll(tempDir+"/"+SignatureDir, 0755)
	defer os.RemoveAll(tempDir)

	global := &mirror.GlobalOptions{
		TlsVerify:    false,
		SecurePolicy: false,
		WorkingDir:   tempDir,
	}

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
		Destination:         "docker://localhost:5000/test",
		Dev:                 false,
		Mode:                mirror.DiskToMirror,
	}

	cfg := v1alpha2.ImageSetConfiguration{
		ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
			Mirror: v1alpha2.Mirror{
				Platform: v1alpha2.Platform{
					Architectures: []string{"amd64"},
					Channels: []v1alpha2.ReleaseChannel{
						{
							Name:       "stable-4.13",
							MinVersion: "4.13.10",
							MaxVersion: "4.13.10",
						},
					},
					Graph: true,
				},
			},
		},
	}

	t.Run("Testing ReleaseSignature - should pass", func(t *testing.T) {
		ex := NewSignatureClient(log, cfg, opts)
		var imgs []v1alpha3.CopyImageSchema
		var newImgs []v1alpha3.CopyImageSchema

		imgs = append(imgs, v1alpha3.CopyImageSchema{
			Source:      "quay.io/openshift-release-dev/ocp-release-4.13.10-x86_64",
			Destination: "localhost:9999/ocp-release:4.13.10-x86_64",
		})

		_, err := ex.GenerateReleaseSignatures(context.Background(), imgs)
		assert.Equal(t, "parsing image digest", err.Error())

		newImgs = append(newImgs, v1alpha3.CopyImageSchema{
			Source:      "quay.io/openshift-release-dev/ocp-release:37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e34531",
			Destination: "localhost:9999/ocp-release:4.13.10-x86_64",
		})

		res, err := ex.GenerateReleaseSignatures(context.Background(), newImgs)
		if err != nil {
			t.Fatal(err)
		}
		assert.Contains(t, res[0].Source, "quay.io/openshift-release-dev/ocp-release:4.11.46-aarch64")

		// signature not found
		newImgs[0].Source = "quay.io/openshift-release-dev/ocp-release:37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e345"
		_, err = ex.GenerateReleaseSignatures(context.Background(), newImgs)
		assert.Equal(t, "no signature found for 37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e345 image quay.io/openshift-release-dev/ocp-release:37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e345", err.Error())

		// write file error
		opts.Global.WorkingDir = "none"
		newImgs[0].Source = "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:37433b71c073c6cbfc8173ec7ab2d99032c8e6d6fe29de06e062d85e33e34531"

		_, err = ex.GenerateReleaseSignatures(context.Background(), newImgs)
		if err != nil {
			t.Fatal(err)
		}

	})
}
