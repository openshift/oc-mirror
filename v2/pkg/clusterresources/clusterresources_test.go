package clusterresources

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

func TestIDMSGenerator(t *testing.T) {
	log := clog.New("trace")

	tmpDir := t.TempDir()
	globalD2M := &mirror.GlobalOptions{
		TlsVerify:    false,
		SecurePolicy: false,
		WorkingDir:   tmpDir + "/working-dir",
		From:         tmpDir,
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOptsD2M := mirror.ImageSrcFlags(globalD2M, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOptsD2M := mirror.ImageDestFlags(globalD2M, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	d2mOpts := mirror.CopyOptions{
		Global:              globalD2M,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOptsD2M,
		DestImage:           destOptsD2M,
		RetryOpts:           retryOpts,
		Destination:         "docker://localhost:5000/test",
		Dev:                 false,
		Mode:                mirror.DiskToMirror,
	}

	cfgd2m := v1alpha2.ImageSetConfiguration{
		ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
			Mirror: v1alpha2.Mirror{
				Platform: v1alpha2.Platform{
					Architectures: []string{"amd64"},
					Channels: []v1alpha2.ReleaseChannel{
						{
							Name:       "stable-4.13",
							MinVersion: "4.13.9",
							MaxVersion: "4.13.10",
						},
					},
				},
			},
		},
	}

	ctx := context.Background()

	imageList := []v1alpha3.CopyImageSchema{
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:6d76ffca7a233213325907bae611e835b49c5b933095be1328351f4f5fc67615",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:6d76ffca7a233213325907bae611e835b49c5b933095be1328351f4f5fc67615",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:6d76ffca7a233213325907bae611e835b49c5b933095be1328351f4f5fc67615",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4c181f5cbea53472acd9695232f77a0933a73f7f40f543cbd48dff00e6f03090",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4c181f5cbea53472acd9695232f77a0933a73f7f40f543cbd48dff00e6f03090",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4c181f5cbea53472acd9695232f77a0933a73f7f40f543cbd48dff00e6f03090",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:ff8ef167b679606b17baf75d94a02589048849b550c4cc17d36506a28f22b29c",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:ff8ef167b679606b17baf75d94a02589048849b550c4cc17d36506a28f22b29c",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:ff8ef167b679606b17baf75d94a02589048849b550c4cc17d36506a28f22b29c",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:d62f2612d3b9618a04ac0dea3ee2e1dec63d8fbe2279e86aa2a605d8755f2b8f",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:d62f2612d3b9618a04ac0dea3ee2e1dec63d8fbe2279e86aa2a605d8755f2b8f",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:d62f2612d3b9618a04ac0dea3ee2e1dec63d8fbe2279e86aa2a605d8755f2b8f",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
		},
	}

	t.Run("Testing IDMSGenerator - Disk to Mirror : should pass", func(t *testing.T) {
		cr := &ClusterResourcesGenerator{
			Log:    log,
			Config: cfgd2m,
			Opts:   d2mOpts,
		}
		err := cr.IDMSGenerator(ctx, imageList, d2mOpts)
		if err != nil {
			t.Fatalf("should not fail")
		}

		_, err = os.Stat(filepath.Join(d2mOpts.Global.WorkingDir, clusterResourcesDir))
		if err != nil {
			t.Fatalf("output folder should exist")
		}

		idmsFiles, err := os.ReadDir(filepath.Join(d2mOpts.Global.WorkingDir, clusterResourcesDir))
		if err != nil {
			t.Fatalf("ls output folder should not fail")
		}

		if len(idmsFiles) != 1 {
			t.Fatalf("output folder should contain 1 idms yaml file")
		}
		// check idmsFile has a name that is
		//compliant with Kubernetes requested
		// RFC-1035 + RFC1123
		// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-label-names
		customResourceName := strings.TrimSuffix(idmsFiles[0].Name(), ".yaml")
		if !isValidRFC1123(customResourceName) {
			t.Fatalf("IDMS custom resource name %s doesn't  respect RFC1123", idmsFiles[0].Name())
		}
	})
}

func isValidRFC1123(name string) bool {
	// Regular expression to match RFC1123 compliant names
	rfc1123Regex := "^[a-zA-Z0-9][-a-zA-Z0-9]*[a-zA-Z0-9]$"
	match, _ := regexp.MatchString(rfc1123Regex, name)
	return match && len(name) <= 63
}

func TestGenerateImageMirrors(t *testing.T) {

	imageList := []v1alpha3.CopyImageSchema{
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:6d76ffca7a233213325907bae611e835b49c5b933095be1328351f4f5fc67615",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:6d76ffca7a233213325907bae611e835b49c5b933095be1328351f4f5fc67615",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:6d76ffca7a233213325907bae611e835b49c5b933095be1328351f4f5fc67615",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4c181f5cbea53472acd9695232f77a0933a73f7f40f543cbd48dff00e6f03090",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4c181f5cbea53472acd9695232f77a0933a73f7f40f543cbd48dff00e6f03090",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:4c181f5cbea53472acd9695232f77a0933a73f7f40f543cbd48dff00e6f03090",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:ff8ef167b679606b17baf75d94a02589048849b550c4cc17d36506a28f22b29c",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:ff8ef167b679606b17baf75d94a02589048849b550c4cc17d36506a28f22b29c",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:ff8ef167b679606b17baf75d94a02589048849b550c4cc17d36506a28f22b29c",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:d62f2612d3b9618a04ac0dea3ee2e1dec63d8fbe2279e86aa2a605d8755f2b8f",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:d62f2612d3b9618a04ac0dea3ee2e1dec63d8fbe2279e86aa2a605d8755f2b8f",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:d62f2612d3b9618a04ac0dea3ee2e1dec63d8fbe2279e86aa2a605d8755f2b8f",
		},
		{
			Source:      "localhost:5000/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Destination: "myregistry/mynamespace/quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
			Origin:      "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c4ef7434c97c8aaf6cd310874790b915b3c61fc902eea255f9177058ea9aff3",
		},
	}

	t.Run("Testing GenerateImageMirrors - Disk to Mirror : should have 1 namespace", func(t *testing.T) {

		mirrors, err := generateImageMirrors(imageList)
		if err != nil {
			t.Fatalf("should not fail")
		}
		if len(mirrors) != 1 {
			t.Fatal("should contain 1 source")
		}

		idm := mirrors["quay.io/openshift-release-dev"]
		if len(idm) != 1 {
			t.Fatalf("should contain 1 mirror for source quay.io/openshift-release-dev. Found %d", len(idm))
		}

		if idm[0] != "myregistry/mynamespace/quay.io/openshift-release-dev" {
			t.Fatalf("returned mirror does not match expected: %s", idm[0])
		}
	})
}
