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
	"github.com/containers/common/pkg/config"
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

func (o *LocalStorageCollector) createGraphImage() (string, error) {
	o.Log.Info("Creating graph image - start")
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

	err = saveWithUidGid(body, graphArchive, 0644, 0, 0)
	// save graph data to tar.gz file modifying UID and GID to root
	//err = os.WriteFile(outputFile, body, 0644)
	if err != nil {
		return "", err
	}
	o.Log.Info("saved cinncinati graph data to tar file")
	// create a container image
	// graphDataUntarFolder := "graph-data-untarred"
	// err = untar(outputFile, graphDataUntarFolder)
	// if err != nil {
	// 	return "", err
	// }

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
	o.Log.Info("creating a storage for buildah builder")

	buildStore, err := storage.GetStore(buildStoreOptions)
	if err != nil {
		return "", err
	}
	o.Log.Info("created a storage for buildah builder")
	defer buildStore.Shutdown(false)

	builderOpts := buildah.BuilderOptions{
		FromImage:    graphBaseImage,
		Capabilities: capabilitiesForRoot,
		Logger:       logger,
	}
	o.Log.Info("Before creating buildah builder")
	builder, err := buildah.NewBuilder(context.TODO(), buildStore, builderOpts)
	if err != nil {
		return "", err
	}
	defer builder.Delete()
	o.Log.Info("Created buildah builder")
	addOptions := buildah.AddAndCopyOptions{Chown: "0:0", PreserveOwnership: true}

	err = builder.Add(graphDataDir, false, addOptions, graphArchive)
	if err != nil {
		return "", err
	}
	o.Log.Info("Added graph data to buildah builder")
	builder.SetCmd([]string{"/bin/bash", "-c", fmt.Sprintf("exec cp -rp %s/* %s", graphDataDir, graphDataMountPath)})
	imageRef, err := alltransports.ParseImageName("docker://" + filepath.Join(o.LocalStorageFQDN, graphImageName))
	if err != nil {
		return "", err
	}

	_, _, digest, err := builder.Commit(context.TODO(), imageRef, buildah.CommitOptions{SystemContext: o.Opts.Global.NewSystemContext()})
	if err != nil {
		return "", err
	}
	return filepath.Join(o.LocalStorageFQDN, graphImageName) + "@" + digest.String(), nil
}
