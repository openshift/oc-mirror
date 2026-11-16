// Package fixtures builds the images used by the integration tests from their sources in
// testdata: operator catalogs, their bundles and operands, release payloads and their
// components. It pushes them to a registry and signs them like podman and skopeo do.
package fixtures

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// fileOwner is the owner of the files copied into a layer. The zero value is root.
type fileOwner struct {
	uid, gid int
}

// dirLayer returns a layer with the contents of srcDir under targetDir, a relative path, or at
// the root of the image when targetDir is empty. targetDir and its parents are owned by root,
// with mode 0755 unless set in dirModes. The copied files keep their permission bits and get
// owner, and timestamps are cleared so the layer only depends on its content.
func dirLayer(srcDir, targetDir string, owner fileOwner, dirModes map[string]int64) (v1.Layer, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	var dir string
	for _, element := range strings.Split(targetDir, "/") {
		if element == "" {
			continue
		}
		dir = path.Join(dir, element)
		mode, ok := dirModes[dir]
		if !ok {
			mode = 0o755
		}
		header := &tar.Header{Typeflag: tar.TypeDir, Name: dir + "/", Mode: mode, ModTime: time.Unix(0, 0)}
		if err := tw.WriteHeader(header); err != nil {
			return nil, err
		}
	}
	err := filepath.WalkDir(srcDir, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, file)
		if err != nil || rel == "." {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header := &tar.Header{
			Name: path.Join(targetDir, filepath.ToSlash(rel)), Mode: int64(info.Mode().Perm()),
			Uid: owner.uid, Gid: owner.gid, ModTime: time.Unix(0, 0),
		}
		switch {
		case entry.IsDir():
			header.Typeflag = tar.TypeDir
			header.Name += "/"
			return tw.WriteHeader(header)
		case info.Mode().IsRegular():
			header.Typeflag = tar.TypeReg
			header.Size = info.Size()
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			f, err := os.Open(file)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(tw, f)
			return err
		default:
			return fmt.Errorf("unsupported file type %s for %s", info.Mode().Type(), file)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add %s to the layer: %w", srcDir, err)
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	data := buf.Bytes()
	return tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	})
}

// layerMediaType returns the media type for a gzip layer added to img, matching its manifest
// format. The images are converted to the format they are published in when they are pushed.
func layerMediaType(img v1.Image) (types.MediaType, error) {
	mediaType, err := img.MediaType()
	if err != nil {
		return "", err
	}
	if mediaType == types.DockerManifestSchema2 {
		return types.DockerLayer, nil
	}
	return types.OCILayer, nil
}

// Index returns an OCI image index with one image per platform, in the given order.
func Index(platforms []v1.Platform, images []v1.Image) (v1.ImageIndex, error) {
	if len(platforms) != len(images) {
		return nil, fmt.Errorf("got %d images for %d platforms", len(images), len(platforms))
	}
	addenda := make([]mutate.IndexAddendum, 0, len(images))
	for i, img := range images {
		mediaType, err := img.MediaType()
		if err != nil {
			return nil, err
		}
		platform := platforms[i]
		addenda = append(addenda, mutate.IndexAddendum{
			Add:        img,
			Descriptor: v1.Descriptor{MediaType: mediaType, Platform: &platform},
		})
	}
	return mutate.AppendManifests(mutate.IndexMediaType(empty.Index, types.OCIImageIndex), addenda...), nil
}
