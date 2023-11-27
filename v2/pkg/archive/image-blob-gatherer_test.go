package archive

import (
	"bufio"
	"context"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
	"github.com/stretchr/testify/assert"
)

func TestImageBlobGatherer_GatherBlobs(t *testing.T) {
	ctx := context.Background()
	global := &mirror.GlobalOptions{
		TlsVerify:    false,
		SecurePolicy: false,
		Force:        true,
		WorkingDir:   "tests",
	}
	global.TlsVerify = false

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
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
	}
	// Set up a fake registry.
	s := httptest.NewServer(registry.New())
	defer s.Close()
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	imageAbsolutePath, err := filepath.Abs("../../tests/noo-bundle-image")
	if err != nil {
		t.Fatal(err)
	}
	src := "dir://" + imageAbsolutePath

	dest := "docker://" + u.Host + "/test:latest"

	err = mirror.New(mirror.NewMirrorCopy(), mirror.NewMirrorDelete()).Run(ctx, src, dest, "copy", &opts, *bufio.NewWriter(os.Stdout))
	// _, err = mirror.CopyImage(ctx, policyContext, destRef, srcRef, co)
	if err != nil {
		t.Fatal(err)
	}
	gatherer := NewImageBlobGatherer(&opts)

	blobs, err := gatherer.GatherBlobs(ctx, "docker://"+u.Host+"/test:latest")
	if err != nil {
		t.Fatalf("GatherBlobs failed: %v", err)
	}

	expectedBlobs := map[string]string{
		"sha256:467829ca4ff134ef9762a8f69647fdf2515b974dfc94a8474c493a45ef922e51": "",
		"sha256:728191dbaae078c825ffb518e15d33956353823d4da6c2e81fe9b1ed60ddef7d": "",
		"sha256:50b9402635dd4b312a86bed05dcdbda8c00120d3789ec2e9b527045100b3bdb4": "",
	}

	assert.Equal(t, expectedBlobs, blobs)
}
