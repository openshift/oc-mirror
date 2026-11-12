package manifest

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// normalizeAbsoluteTarPaths rewrites OCI layer entries whose names start with
// "/" so go-containerregistry can apply whiteouts to them.
//
// Catalogs filtered by older oc-mirror builds delete the previous configs with
// a whiteout named "/.wh.configs". mutate.Extract records that tombstone as
// "/configs", which does not cover the relative path "configs/..." used by the
// other layers. The deleted catalog is therefore extracted together with the
// filtered one, and the next rebuild writes duplicate channels.
func normalizeAbsoluteTarPaths(img gcrv1.Image) (gcrv1.Image, error) {
	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("listing image layers: %w", err)
	}

	normalized := make([]gcrv1.Layer, len(layers))
	changed := false
	for i, layer := range layers {
		absolute, err := layerHasAbsolutePath(layer)
		if err != nil {
			return nil, err
		}
		if !absolute {
			normalized[i] = layer
			continue
		}
		rewritten, err := stripAbsoluteTarPaths(layer)
		if err != nil {
			return nil, err
		}
		normalized[i] = rewritten
		changed = true
	}
	if !changed {
		return img, nil
	}

	// Extract only needs the filesystem. Rebuilding from the normalized layers
	// drops history that is irrelevant to whiteout handling.
	return mutate.AppendLayers(empty.Image, normalized...)
}

func layerHasAbsolutePath(layer gcrv1.Layer) (bool, error) {
	rc, err := layer.Uncompressed()
	if err != nil {
		return false, fmt.Errorf("opening layer: %w", err)
	}
	defer rc.Close()

	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("reading layer tar: %w", err)
		}
		if strings.HasPrefix(hdr.Name, "/") {
			return true, nil
		}
	}
}

func stripAbsoluteTarPaths(layer gcrv1.Layer) (gcrv1.Layer, error) {
	rc, err := layer.Uncompressed()
	if err != nil {
		return nil, fmt.Errorf("opening layer: %w", err)
	}
	defer rc.Close()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading layer tar: %w", err)
		}

		name := strings.TrimLeft(hdr.Name, "/")
		if name == "" || name == "." {
			continue
		}
		hdr.Name = name
		if hdr.PAXRecords != nil {
			if _, ok := hdr.PAXRecords["path"]; ok {
				hdr.PAXRecords["path"] = name
			}
		}

		if err := tw.WriteHeader(hdr); err != nil {
			return nil, fmt.Errorf("writing tar header %q: %w", name, err)
		}
		if hdr.Size > 0 {
			if _, err := io.CopyN(tw, tr, hdr.Size); err != nil {
				return nil, fmt.Errorf("writing tar file %q: %w", name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("closing tar: %w", err)
	}

	uncompressed := buf.Bytes()
	rewritten, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(uncompressed)), nil
	})
	if err != nil {
		return nil, fmt.Errorf("creating normalized layer: %w", err)
	}
	return rewritten, nil
}
