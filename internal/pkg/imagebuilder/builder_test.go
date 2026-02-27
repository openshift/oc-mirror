package imagebuilder

import (
	"context"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/layout"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

const (
	graphDataDir       = "/var/lib/cincinnati-graph-data"
	graphDataMountPath = "/var/lib/cincinnati/graph-data"
	graphArchive       = "cincinnati-graph-data.tar"
	graphStaging       = consts.TestFolder + "graph-staging/"
	graphPreparation   = consts.TestFolder + "graph-preparation/"
)

func TestImageBuilder(t *testing.T) {

	log := clog.New("trace")

	tempDir := t.TempDir()

	global := &mirror.GlobalOptions{
		SecurePolicy: false,
		WorkingDir:   tempDir + "/working-dir",
		From:         tempDir,
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	srcFlags, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	destFlags, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	_ = srcFlags.Set("src-tls-verify", "false")
	_ = destFlags.Set("dest-tls-verify", "false")

	opts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Destination:         consts.DockerProtocol + "localhost:5000/test",
		Dev:                 false,
		Mode:                mirror.DiskToMirror,
		All:                 true,
	}

	t.Run("Testing NewImageBuilder : should pass", func(t *testing.T) {

		_ = NewBuilder(log, opts)
		_ = srcFlags.Set("src-tls-verify", "true")
		_ = destFlags.Set("dest-tls-verify", "true")
		_ = NewBuilder(log, opts)

		e := ErrInvalidReference{image: "broken"}
		log.Error("testing error %v", e.Error())

	})

	t.Run("Testing NewImageBuilder All : should pass", func(t *testing.T) {

		// Set up a fake registry.
		s := httptest.NewServer(registry.New())
		defer s.Close()
		u, err := url.Parse(s.URL)
		if err != nil {
			t.Fatal(err)
		}

		_ = srcFlags.Set("src-tls-verify", "false")
		_ = destFlags.Set("dest-tls-verify", "false")
		ex := NewBuilder(log, opts)
		ctx := context.Background()

		// expect errors from LayerFromGzipByteArray
		archiveDestination := consts.TestFolder + "graph-staging/" + graphArchive
		_, err = LayerFromGzipByteArray([]byte{}, archiveDestination, graphDataDir, 0644, 0, 0)
		if err == nil {
			t.Fatalf("should fail")
		}

		body, err := os.ReadFile(consts.TestFolder + "graph-assets/graph-data.tar.gz")
		if err != nil {
			t.Fatal(err)
		}

		// expect errors from LayerFromGzipByteArray
		archiveDestination = "/no-dir/" + graphArchive
		_, err = LayerFromGzipByteArray(body, archiveDestination, graphDataDir, 0644, 0, 0)
		if err == nil {
			t.Fatalf("should fail")
		}

		archiveDestination = consts.TestFolder + "graph-staging/" + graphArchive
		graphLayer, err := LayerFromGzipByteArray(body, archiveDestination, graphDataDir, 0644, 0, 0)
		if err != nil {
			t.Fatal(err)
		}

		// expect errors from SaveImageLayout
		_, err = ex.SaveImageLayoutToDir(context.Background(), "broken", graphPreparation)
		if err == nil {
			t.Fatalf("should fail")
		}

		// setup graphPreparation
		_ = os.Mkdir(graphPreparation, 0755)
		// ensure directories are cleaned up after test
		defer os.RemoveAll(opts.Global.WorkingDir)
		defer os.Remove(archiveDestination)
		defer os.RemoveAll(graphPreparation)

		imageAbsolutePath, err := filepath.Abs(consts.TestFolder + "simple-test-bundle")
		if err != nil {
			t.Fatal(err)
		}

		src := consts.OciProtocol + imageAbsolutePath
		dest := consts.DockerProtocol + u.Host + "/simple-test-bundle:latest"

		err = mirror.New(mirror.NewMirrorCopy(), mirror.NewMirrorDelete()).Run(ctx, src, dest, "copy", &opts)
		if err != nil {
			t.Fatal(err)
		}

		// use lightweight simple image reference for testing
		lp, err := ex.SaveImageLayoutToDir(context.Background(), u.Host+"/simple-test-bundle:latest", graphPreparation)
		if err != nil {
			t.Fatal(err)
		}

		_, err = ex.BuildAndPush(ctx, u.Host+"/new-build:latest", lp, []string{"ls -la"}, graphLayer)
		if err != nil {
			t.Fatal(err)
		}

	})
}

func TestProcessImageIndex(t *testing.T) {
	t.Run("Testing ProcessImageIndex: should pass", func(t *testing.T) {

		log := clog.New("debug")

		global := &mirror.GlobalOptions{
			SecurePolicy: false,
		}

		_, sharedOpts := mirror.SharedImageFlags()
		_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
		srcFlags, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
		destFlags, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
		_, retryOpts := mirror.RetryFlags()

		_ = srcFlags.Set("src-tls-verify", "false")
		_ = destFlags.Set("dest-tls-verify", "false")

		opts := mirror.CopyOptions{
			Global:              global,
			DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
			SrcImage:            srcOpts,
			DestImage:           destOpts,
			RetryOpts:           retryOpts,
			Destination:         consts.DockerProtocol + "localhost:5000/test",
			Dev:                 false,
			Mode:                mirror.DiskToMirror,
		}

		ex := NewBuilder(log, opts)
		ctx := context.Background()

		body, err := os.ReadFile(consts.TestFolder + "graph-assets/graph-data.tar.gz")
		if err != nil {
			t.Fatal(err)
		}

		archiveDestination := consts.TestFolder + "graph-staging/" + graphArchive
		graphLayer, err := LayerFromGzipByteArray(body, archiveDestination, graphDataDir, 0644, 0, 0)
		if err != nil {
			t.Fatal(err)
		}

		v2format := false

		// cover the mediatype as list
		idx, err := layout.ImageIndexFromPath(consts.TestFolder + "test-process-image-list/")
		if err != nil {
			t.Fatal(err)
		}

		_, err = ex.ProcessImageIndex(ctx, idx, &v2format, []string{"ls -la"}, "localhost:5000/new-build:latest", graphLayer)
		if err != nil {
			t.Fatal(err)
		}

		// cover the mediatype as oci - normal
		idx, err = layout.ImageIndexFromPath(consts.TestFolder + "test-process-image/")
		if err != nil {
			t.Fatal(err)
		}

		_, err = ex.ProcessImageIndex(ctx, idx, &v2format, []string{"ls -la"}, "localhost:5000/new-build:latest", graphLayer)
		if err != nil {
			t.Fatal(err)
		}

		// cover the mediatype as oci - normal
		idx, err = layout.ImageIndexFromPath(consts.TestFolder + "test-process-image-bad-platform/")
		if err != nil {
			t.Fatal(err)
		}

		_, err = ex.ProcessImageIndex(ctx, idx, &v2format, []string{"ls -la"}, "localhost:5000/new-build:latest", graphLayer)
		if err != nil {
			t.Fatal(err)
		}

		// cover the mediatype as unsupported
		idx, err = layout.ImageIndexFromPath(consts.TestFolder + "test-process-image-unsupported/")
		if err != nil {
			t.Fatal(err)
		}

		_, err = ex.ProcessImageIndex(ctx, idx, &v2format, []string{"ls -la"}, "localhost:5000/new-build:latest", graphLayer)
		if err == nil {
			t.Fatalf("should fail")
		}

		defer os.RemoveAll(opts.Global.WorkingDir)
		defer os.Remove(archiveDestination)

	})
}
