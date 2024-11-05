package imagebuilder

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/otiai10/copy"
)

const (
	// Mode constants from the USTAR spec:
	// See http://pubs.opengroup.org/onlinepubs/9699919799/utilities/pax.html#tag_20_92_13_06
	c_ISUID = 04000 // Set uid
	c_ISGID = 02000 // Set gid
	c_ISVTX = 01000 // Save text (sticky bit)

	indexSubFolder = "operator-filtered-index"
)

type GCRCatalogBuilder struct {
	CatalogBuilderInterface
	imgBuilder ImageBuilderInterface
}

func NewGCRCatalogBuilder(logger log.PluggableLoggerInterface, opts mirror.CopyOptions) CatalogBuilderInterface {
	builder := NewBuilder(logger, opts)
	return &GCRCatalogBuilder{
		imgBuilder: builder,
	}
}

func (c GCRCatalogBuilder) RebuildCatalog(ctx context.Context, catalogCopyRef v2alpha1.CopyImageSchema, configPath string) (v2alpha1.CopyImageSchema, error) {
	layersToAdd := []v1.Layer{}
	layersToDelete := []v1.Layer{}

	_, err := os.Stat(configPath)
	if err != nil {
		return catalogCopyRef, fmt.Errorf("error reading filtered config for catalog %s from %s: %v", catalogCopyRef.Origin, configPath, err)
	}

	originCatalogLayoutDir := strings.Replace(configPath, "filtered-operator", "operator-images", -1)
	_, err = os.Stat(originCatalogLayoutDir)
	if err != nil {
		return catalogCopyRef, fmt.Errorf("error initializing a container image for catalog %s from %s: %v", catalogCopyRef.Origin, originCatalogLayoutDir, err)
	}

	configLayerToAdd, err := LayerFromPathWithUidGid("/configs", configPath, 0, 0)
	if err != nil {
		return catalogCopyRef, fmt.Errorf("error creating add layer: %v", err)
	}
	layersToAdd = append(layersToAdd, configLayerToAdd)

	// Since we are defining the FBC as index.json,
	// remove anything that may currently exist
	deletedConfigLayer, err := deleteLayer("/.wh.configs")
	if err != nil {
		return catalogCopyRef, fmt.Errorf("error preparing to delete old /configs from catalog %s : %v", catalogCopyRef.Origin, err)
	}
	layersToDelete = append(layersToDelete, deletedConfigLayer)

	// Deleted layers must be added first in the slice
	// so that the /configs and /tmp directories are deleted
	// and then added back from the layers rebuilt from the new FBC.
	layers := []v1.Layer{}
	layers = append(layers, layersToDelete...)
	layers = append(layers, layersToAdd...)

	layoutDir := strings.Replace(configPath, "filtered-operator", indexSubFolder, -1)

	err = copy.Copy(originCatalogLayoutDir, layoutDir)
	if err != nil {
		return catalogCopyRef, fmt.Errorf("error creating OCI layout: %v", err)
	}
	layoutPath, err := layout.FromPath(layoutDir)
	if err != nil {
		return catalogCopyRef, fmt.Errorf("error creating OCI layout: %v", err)
	}

	configCMD := []string{"serve", "/configs"}

	if err := c.imgBuilder.BuildAndPush(ctx, strings.TrimPrefix(catalogCopyRef.Destination, dockerProtocol), layoutPath, configCMD, layers...); err != nil {
		return catalogCopyRef, fmt.Errorf("error building catalog %s : %v", catalogCopyRef.Origin, err)
	}
	return catalogCopyRef, nil
}

// LayerFromPath will write the contents of the path(s) the target
// directory specifying the target UID/GID and build a v1.Layer.
// Use gid = -1 , uid = -1 if you don't want to override.
func LayerFromPathWithUidGid(targetPath, path string, uid int, gid int) (v1.Layer, error) {
	var b bytes.Buffer
	tw := tar.NewWriter(&b)

	pathInfo, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	processPaths := func(hdr *tar.Header, info os.FileInfo, fp string) error {
		if !info.IsDir() {
			hdr.Size = info.Size()
		}
		hdr.ChangeTime = time.Now()
		if info.Mode().IsDir() {
			hdr.Typeflag = tar.TypeDir
		} else if info.Mode().IsRegular() {
			hdr.Typeflag = tar.TypeReg
		} else {
			return fmt.Errorf("not implemented archiving file type %s (%s)", info.Mode(), info.Name())
		}

		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("failed to write tar header: %w", err)
		}
		if !info.IsDir() {
			f, err := os.Open(filepath.Clean(fp))
			if err != nil {
				return err
			}
			if _, err := io.Copy(tw, f); err != nil {
				return fmt.Errorf("failed to read file into the tar: %w", err)
			}
			err = f.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}

	if pathInfo.IsDir() {
		err := filepath.Walk(path, func(fp string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(path, fp)
			if err != nil {
				return fmt.Errorf("failed to calculate relative path: %w", err)
			}

			hdr := &tar.Header{
				Name:   filepath.Join(targetPath, filepath.ToSlash(rel)),
				Format: tar.FormatPAX,
				Mode:   int64(info.Mode().Perm()),
			}
			if uid != -1 {
				hdr.Uid = uid
			}
			if gid != -1 {
				hdr.Gid = gid
			}

			if info.Mode()&os.ModeSetuid != 0 {
				hdr.Mode |= c_ISUID
			}
			if info.Mode()&os.ModeSetgid != 0 {
				hdr.Mode |= c_ISGID
			}
			if info.Mode()&os.ModeSticky != 0 {
				hdr.Mode |= c_ISVTX
			}

			if err := processPaths(hdr, info, fp); err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to scan files: %w", err)
		}

	} else {
		base := filepath.Base(path)
		hdr := &tar.Header{
			Name:   filepath.Join(targetPath, filepath.ToSlash(base)),
			Format: tar.FormatPAX,
			Mode:   int64(pathInfo.Mode().Perm()),
		}
		if uid != -1 { // uid was specified in the input param
			hdr.Uid = uid
		}
		if gid != -1 { // gid was specified in the input param
			hdr.Gid = gid
		}

		if pathInfo.Mode()&os.ModeSetuid != 0 {
			hdr.Mode |= c_ISUID
		}
		if pathInfo.Mode()&os.ModeSetgid != 0 {
			hdr.Mode |= c_ISGID
		}
		if pathInfo.Mode()&os.ModeSticky != 0 {
			hdr.Mode |= c_ISVTX
		}

		if err := processPaths(hdr, pathInfo, path); err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("failed to finish tar: %w", err)
	}

	opener := func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(b.Bytes())), nil
	}
	return tarball.LayerFromOpener(opener)
}

func deleteLayer(old string) (v1.Layer, error) {
	deleteMap := map[string][]byte{}
	deleteMap[old] = []byte{}
	return crane.Layer(deleteMap)
}
