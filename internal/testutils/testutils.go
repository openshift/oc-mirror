package testutils

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/distribution/manifest"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// WriteTestImage will use go-containerregistry to push a test image to
// an httptest.Server and will write the image to an OCI layout if dir is not "".
func WriteTestImage(testServer *httptest.Server, dir string) (string, error) {
	return WriteTestImageWithURL(testServer.URL, dir)
}

// WriteTestImageWithURL is similar to WriteTestImage, but is useful when testing
// with external registries rather than a httptest.Server
func WriteTestImageWithURL(URL string, dir string) (string, error) {
	u, err := url.Parse(URL)
	if err != nil {
		return "", err
	}
	c := map[string][]byte{
		"/testfile": []byte("test contents contents"),
	}
	targetRef := fmt.Sprintf("%s/bar:foo", u.Host)
	tag, err := name.NewTag(targetRef)
	if err != nil {
		return "", err
	}
	i, _ := crane.Image(c)

	if dir != "" {
		// create a new layout.Path and append the image to it
		lp, err := layout.Write(dir, empty.Index)
		if err != nil {
			return "", err
		}
		// write the image into the layout path
		if err := lp.AppendImage(i); err != nil {
			return "", err
		}
		// reload the path as an index and "push" it to remote
		idx, err := lp.ImageIndex()
		if err != nil {
			return "", err
		}
		if err := remote.WriteIndex(tag, idx); err != nil {
			return "", err
		}
	} else {
		// push image to remote
		if err := crane.Push(i, tag.String()); err != nil {
			return "", err
		}
	}
	return targetRef, nil
}

// WriteMultiarchTestImageWithURL will use go-containerregistry to push a multi arch test image to
// an httptest.Server and will write the image to an OCI layout if dir is not "".
func WriteMultiArchTestImage(testServer *httptest.Server, dir string) (string, error) {
	return WriteMultiArchTestImageWithURL(testServer.URL, dir)
}

// WriteMultiArchTestImageWithURL is similar to WriteMultiArchTestImage, but is useful when testing
// with external registries rather than a httptest.Server
func WriteMultiArchTestImageWithURL(URL string, dir string) (string, error) {
	u, err := url.Parse(URL)
	if err != nil {
		return "", err
	}
	targetRef := fmt.Sprintf("%s/bar:foo", u.Host)
	tag, err := name.NewTag(targetRef)
	if err != nil {
		return "", err
	}
	makeLayer := func() v1.Layer {
		layer, _ := random.Layer(100, types.DockerLayer)
		if err != nil {
			// this is a hack, but it should not happen
			panic(err)
		}
		return layer
	}
	img1, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64"})
	if err != nil {
		return "", err
	}
	img1, err = mutate.Append(img1, mutate.Addendum{
		Layer: makeLayer(),
		History: v1.History{
			Author:    "random.Image",
			Comment:   fmt.Sprintf("this is a random history %d of %d", 1, 1),
			CreatedBy: "random",
			Created:   v1.Time{Time: time.Now()},
		},
		// MediaType: types.DockerManifestSchema2,
	})
	img2, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "ppc64le"})
	if err != nil {
		return "", err
	}
	img2, err = mutate.Append(img2, mutate.Addendum{
		Layer: makeLayer(),
		History: v1.History{
			Author:    "random.Image",
			Comment:   fmt.Sprintf("this is a random history %d of %d", 1, 1),
			CreatedBy: "random",
			Created:   v1.Time{Time: time.Now()},
		},
		// MediaType: types.DockerManifestSchema2,
	})
	ii := mutate.AppendManifests(empty.Index,
		mutate.IndexAddendum{
			Add: img1,
			Descriptor: v1.Descriptor{
				Platform: &v1.Platform{
					OS:           "linux",
					Architecture: "amd64",
				},
			},
		},
		mutate.IndexAddendum{
			Add: img2,
			Descriptor: v1.Descriptor{
				Platform: &v1.Platform{
					OS:           "linux",
					Architecture: "ppc64le",
				},
			},
		},
	)
	// update the MediaType for this index
	ii = mutate.IndexMediaType(ii, types.DockerManifestList)
	if dir != "" {
		// "wrap" the newly created index in a new index (i.e. create manifest list indirection)
		// NOTE: manifest list indirection is only useful for OCI layouts... this is
		// not supported when pushing to a remote registry
		oci_ii := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{
			Add: ii,
			Descriptor: v1.Descriptor{
				MediaType: types.DockerManifestList,
			},
		})
		_, err := layout.Write(dir, oci_ii)
		if err != nil {
			return "", err
		}
	}
	// now "push" to remote
	if err := remote.WriteIndex(tag, ii); err != nil {
		return "", err
	}
	return targetRef, nil
}

// RegistryFromFiles mirror a local V2 directory at the destintation with the source directory.
func RegistryFromFiles(source string) http.HandlerFunc {
	dir := http.Dir(source)
	fileHandler := http.FileServer(dir)
	handler := func(w http.ResponseWriter, req *http.Request) {
		if req.Method == "GET" && req.URL.Path == "/v2/" {
			w.Header().Set("Docker-Distribution-API-Version", "2.0")
		}
		if req.Method == "GET" {
			switch path.Base(path.Dir(req.URL.Path)) {
			case "blobs":
				w.Header().Set("Content-Type", "application/octet-stream")
			case "manifests":
				if f, err := dir.Open(req.URL.Path); err == nil {
					defer f.Close()
					if data, err := io.ReadAll(f); err == nil {
						var versioned manifest.Versioned
						if err = json.Unmarshal(data, &versioned); err == nil {
							w.Header().Set("Content-Type", versioned.MediaType)
						}
					}
				}
			}
		}
		fileHandler.ServeHTTP(w, req)
	}
	return http.HandlerFunc(handler)
}

// LocalMirrorFromFiles copies a local V2 directory to the destintation with the source directory.
func LocalMirrorFromFiles(source string, destination string) error {
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		relPath := strings.Replace(path, source, "", 1)
		if relPath == "" {
			return nil
		}
		switch m := info.Mode(); {
		case m&fs.ModeSymlink != 0: // Tag is the file name, so follow the symlink to the layer ID-named file.
			dst, err := os.Readlink(path)
			if err != nil {
				return err
			}
			id := filepath.Base(dst)
			if err := os.Symlink(id, filepath.Join(destination, relPath)); err != nil {
				return err
			}
		case m.IsDir():
			return os.Mkdir(filepath.Join(destination, relPath), 0750)
		default:
			newSource := filepath.Join(source, relPath)
			cleanSource := filepath.Clean(newSource)
			data, err := os.ReadFile(cleanSource)
			if err != nil {
				return err
			}
			newDest := filepath.Join(destination, relPath)
			cleanDest := filepath.Clean(newDest)
			return os.WriteFile(cleanDest, data, 0600)
		}
		return nil
	})
	return err
}
