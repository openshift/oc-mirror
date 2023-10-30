package release

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/openshift/oc-mirror/v2/pkg/imagebuilder"
)

// createGraphImage creates a graph image from the graph data
// and returns the image reference.
// it follows https://docs.openshift.com/container-platform/4.13/updating/updating-restricted-network-cluster/restricted-network-update-osus.html#update-service-graph-data_updating-restricted-network-cluster-osus
func (o *LocalStorageCollector) CreateGraphImage(ctx context.Context) (string, error) {
	// HTTP Get the graph updates from api endpoint
	resp, err := http.Get(graphURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// save graph data in a container layer modifying UID and GID to root.
	archiveDestination := filepath.Join(o.Opts.Global.Dir, graphArchive)
	graphLayer, err := imagebuilder.LayerFromGzipByteArray(body, archiveDestination, graphDataDir, 0644, 0, 0)
	if err != nil {
		return "", err
	}
	defer os.Remove(archiveDestination)

	// Create a local directory for saving the OCI image layout of UBI9
	layoutDir := filepath.Join(o.Opts.Global.Dir, graphPreparationDir)
	if err := os.MkdirAll(layoutDir, os.ModePerm); err != nil {
		return "", err
	}

	// Create an imageBuilder
	nameOptions, remoteOptions := setImgBuilderDefaultOptions(ctx)
	imgBuilder := imagebuilder.NewBuilder(nameOptions, remoteOptions, o.Log)

	// Use the imgBuilder to pull the ubi9 image to layoutDir
	layoutPath, err := imgBuilder.SaveImageLayoutToDir(ctx, graphBaseImage, layoutDir)
	if err != nil {
		return "", err
	}

	// preprare the CMD to []string{"/bin/bash", "-c", fmt.Sprintf("exec cp -rp %s/* %s", graphDataDir, graphDataMountPath)}
	cmd := []string{"/bin/bash", "-c", fmt.Sprintf("exec cp -rp %s/* %s", graphDataDir, graphDataMountPath)}

	// update a ubi9 image with this new graphLayer and new cmd
	err = imgBuilder.BuildAndPush(ctx, filepath.Join(o.LocalStorageFQDN, graphImageName)+":latest", layoutPath, cmd, graphLayer)
	if err != nil {
		return "", err
	}
	return "", nil
}

func setImgBuilderDefaultOptions(ctx context.Context) ([]name.Option, []remote.Option) {
	// preparing name options for pulling the ubi9 image:
	// - no need to set defaultRegistry because we are using a fully qualified image name
	// - no need to set insecure to true: ubi image is retrieved from a secure registry
	nameOptions := []name.Option{name.StrictValidation}

	remoteOptions := []remote.Option{
		remote.WithTransport(remote.DefaultTransport),      // no need to create our own roundTripper to pass insecure=true: the registry is secure
		remote.WithAuthFromKeychain(authn.DefaultKeychain), // this will try to find .docker/config first, $XDG_RUNTIME_DIR/containers/auth.json second
		remote.WithContext(ctx),
		// doesn't seem possible to use registries.conf here.
		// TODO test what happens if registries.conf is specified
	}
	return nameOptions, remoteOptions
}
