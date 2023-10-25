package release

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"archive/tar"
	"compress/gzip"

	"github.com/containers/buildah"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/storage"
	"github.com/sirupsen/logrus"
)

func saveWithUidGid(content []byte, outputFile string, mod int, uid, gid int) error {
	gzipReader, err := gzip.NewReader(bytes.NewReader(content))
	defer gzipReader.Close()
	if err != nil {
		return err
	}
	tarReader := tar.NewReader(gzipReader)

	f, err := os.Create(outputFile)
	defer f.Close()
	if err != nil {
		return err
	}
	tarWriter := tar.NewWriter(f)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		header.Uid = uid
		header.Gid = gid

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if _, err := io.Copy(tarWriter, tarReader); err != nil {
			return err
		}
	}

	if err := tarWriter.Close(); err != nil {
		return err
	}

	if err := os.Chmod(outputFile, os.FileMode(mod)); err != nil {
		return err
	}

	return nil
}

// createGraphImage creates a graph image from the graph data
// and returns the image reference.
// it follows https://docs.openshift.com/container-platform/4.13/updating/updating-restricted-network-cluster/restricted-network-update-osus.html#update-service-graph-data_updating-restricted-network-cluster-osus
func (o *LocalStorageCollector) createGraphImage() (string, error) {
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

	// save graph data to tar file modifying UID and GID to root.
	// tar file needs to be in current working directory, so that
	// buildah can add it to the image
	err = saveWithUidGid(body, graphArchive, 0644, 0, 0)
	if err != nil {
		return "", err
	}
	defer os.Remove(graphArchive)

	// Begin buildah setup
	// Buildah's builder needs a storage.Store to work with:
	// intermediate and result images are stored there.
	// Done following https://github.com/containers/buildah/blob/main/docs/tutorials/04-include-in-your-build-tool.md
	buildStoreOptions, err := storage.DefaultStoreOptionsAutoDetectUID()
	if err != nil {
		return "", err
	}
	buildStore, err := storage.GetStore(buildStoreOptions)
	if err != nil {
		return "", err
	}
	defer buildStore.Shutdown(false)

	logger := logrus.New()
	if o.Opts.Global.LogLevel == "debug" {
		logger.Level = logrus.DebugLevel
	}
	builderOpts := buildah.BuilderOptions{
		FromImage: graphBaseImage,
		Logger:    logger,
	}
	builder, err := buildah.NewBuilder(context.TODO(), buildStore, builderOpts)
	if err != nil {
		return "", err
	}
	defer builder.Delete()
	// End buildah setup

	// While adding the layer, we want to preserve the ownership of the files:
	// tar file contents are owned by root, so we need to preserve that.
	addOptions := buildah.AddAndCopyOptions{Chown: "0:0", PreserveOwnership: true}

	// Adding graphArchive contents to the image under /var/lib/cincinnati-graph-data/
	// Second parameter instructs the builder to extract the tar file contents before adding them
	err = builder.Add(graphDataDir, true, addOptions, graphArchive)
	if err != nil {
		return "", err
	}

	// Update the CMD of the image, according to the documentation
	builder.SetCmd([]string{"/bin/bash", "-c", fmt.Sprintf("exec cp -rp %s/* %s", graphDataDir, graphDataMountPath)})

	//Preparing the image reference to build to
	imageRef, err := alltransports.ParseImageName("docker://" + filepath.Join(o.LocalStorageFQDN, graphImageName))
	if err != nil {
		return "", err
	}

	// Run commit the build: this will push the image to the imgRef, instead of only to the buidah storage
	_, _, digest, err := builder.Commit(context.TODO(), imageRef, buildah.CommitOptions{SystemContext: o.Opts.Global.NewSystemContext()})
	if err != nil {
		return "", err
	}
	return filepath.Join(o.LocalStorageFQDN, graphImageName) + "@" + digest.String(), nil
}
