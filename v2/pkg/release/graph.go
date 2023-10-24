package release

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"archive/tar"
	"compress/gzip"

	"github.com/containers/buildah"
	"github.com/containers/common/pkg/config"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/storage"
	"github.com/sirupsen/logrus"
)

func untar(src, dest string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		path := filepath.Join(dest, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			defer file.Close()

			if _, err := io.Copy(file, tarReader); err != nil {
				return err
			}
		}
	}

	return nil
}

func createGraphImage(localStorageFQDN string) (string, error) {

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

	// save graph data to tar.gz file
	err = os.WriteFile(outputFile, body, 0644)
	if err != nil {
		return "", err
	}

	// create a container image
	graphDataUntarFolder := "graph-data-untarred"
	err = untar(outputFile, graphDataUntarFolder)
	if err != nil {
		return "", err
	}

	logger := logrus.New()
	//TODO take log level from localStoreCollection options
	logger.Level = logrus.DebugLevel
	buildStoreOptions, err := storage.DefaultStoreOptionsAutoDetectUID()
	if err != nil {
		return "", err
	}

	conf, err := config.Default()
	if err != nil {
		return "", err
	}

	capabilitiesForRoot, err := conf.Capabilities("root", nil, nil)
	if err != nil {
		return "", err
	}
	buildStore, err := storage.GetStore(buildStoreOptions)
	if err != nil {
		return "", err
	}
	defer buildStore.Shutdown(false)

	builderOpts := buildah.BuilderOptions{
		FromImage:    graphBaseImage,
		Capabilities: capabilitiesForRoot,
		Logger:       logger,
	}
	builder, err := buildah.NewBuilder(context.TODO(), buildStore, builderOpts)
	if err != nil {
		return "", err
	}
	defer builder.Delete()

	addOptions := buildah.AddAndCopyOptions{PreserveOwnership: true}

	err = builder.Add(graphDataDir, false, addOptions, graphDataUntarFolder)
	if err != nil {
		return "", err
	}
	builder.SetCmd([]string{"/bin/bash", "-c", fmt.Sprintf("exec cp -rp %s/* %s", graphDataDir, graphDataMountPath)})
	imageRef, err := alltransports.ParseImageName("docker://" + filepath.Join(localStorageFQDN, graphImageName))
	if err != nil {
		return "", err
	}

	_, _, digest, err := builder.Commit(context.TODO(), imageRef, buildah.CommitOptions{})
	if err != nil {
		return "", err
	}
	return filepath.Join(localStorageFQDN, graphImageName) + "@" + digest.String(), nil
}
