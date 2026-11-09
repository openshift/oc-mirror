package release

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"go.podman.io/image/v5/types"

	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"

	"github.com/openshift/oc-mirror/v2/internal/pkg/imagebuilder"
)

// createGraphImage creates a graph image from the graph data
// and returns the image reference.
// it follows https://docs.openshift.com/container-platform/4.13/updating/updating-restricted-network-cluster/restricted-network-update-osus.html#update-service-graph-data_updating-restricted-network-cluster-osus
func (o *LocalStorageCollector) CreateGraphImage(ctx context.Context, url string) (string, error) {
	// HTTP Get the graph updates from api endpoint
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// save graph data in a container layer modifying UID and GID to root.
	archiveDestination := filepath.Join(o.Opts.Global.WorkingDir, graphArchive)
	graphLayer, err := imagebuilder.LayerFromGzipByteArray(body, archiveDestination, buildGraphDataDir, 0644, 0, 0)
	if err != nil {
		return "", err
	}
	defer os.Remove(archiveDestination)

	// Create a local directory for saving the OCI image layout of UBI9
	layoutDir := filepath.Join(o.Opts.Global.WorkingDir, graphPreparationDir)
	if err := os.MkdirAll(layoutDir, os.ModePerm); err != nil {
		return "", err
	}

	// Use the imgBuilder to pull the ubi9 image to layoutDir
	layoutPath, err := o.ImageBuilder.SaveImageLayoutToDir(ctx, graphBaseImage, layoutDir)
	if err != nil {
		return "", err
	}

	// preprare the CMD to []string{"/bin/bash", "-c", fmt.Sprintf("exec cp -rp %s/* %s", graphDataDir, graphDataMountPath)}
	cmd := []string{"/bin/bash", "-c", fmt.Sprintf("exec cp -rp %s/* %s", buildGraphDataDir, graphDataMountPath)}

	// update a ubi9 image with this new graphLayer and new cmd
	graphImageRef := filepath.Join(o.destinationRegistry(), graphImageName) + ":latest"
	_, err = o.ImageBuilder.BuildAndPush(ctx, graphImageRef, layoutPath, cmd, graphLayer)
	if err != nil {
		return "", err
	}
	return consts.DockerProtocol + graphImageRef, nil
}

func (o *LocalStorageCollector) graphImageInWorkingDir(ctx context.Context) (string, error) {
	layoutDir := filepath.Join(o.Opts.Global.WorkingDir, graphPreparationDir)
	graphImageRef := consts.OciProtocol + layoutDir

	exists, err := o.imageExists(ctx, graphImageRef)
	if err != nil {
		return "", fmt.Errorf("no oci formatted graph image ready in cache: %v", err)
	}
	if !exists {
		return "", fmt.Errorf("no oci formatted graph image ready in cache")
	}
	return graphImageRef, nil
}

func (o LocalStorageCollector) imageExists(ctx context.Context, ref string) (bool, error) {
	sourceCtx, err := o.Opts.SrcImage.NewSystemContext()
	if err != nil {
		return false, err
	}
	// local cache is http protocol
	sourceCtx.DockerInsecureSkipTLSVerify = types.OptionalBoolTrue
	digest, err := o.Manifest.ImageDigest(ctx, sourceCtx, ref)
	if err != nil {
		return false, err
	}
	if digest == "" {
		return false, nil
	}
	return true, nil
}
