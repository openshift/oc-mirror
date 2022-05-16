package testutils

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/docker/distribution/manifest"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// WriteTestImage will use go-containerregistry to push a test image to
// an httptest.Server and will write the image to an OCI layout if dir is not "".
func WriteTestImage(testServer *httptest.Server, dir string) (string, error) {
	u, err := url.Parse(testServer.URL)
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
	if err := crane.Push(i, tag.String()); err != nil {
		return "", err
	}
	if dir != "" {
		lp, err := layout.Write(dir, empty.Index)
		if err != nil {
			return "", err
		}
		if err := lp.AppendImage(i); err != nil {
			return "", err
		}
		idx, err := lp.ImageIndex()
		if err != nil {
			return "", err
		}
		if err := remote.WriteIndex(tag, idx); err != nil {
			return "", err
		}
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
					if data, err := ioutil.ReadAll(f); err == nil {
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
			data, err := ioutil.ReadFile(cleanSource)
			if err != nil {
				return err
			}
			newDest := filepath.Join(destination, relPath)
			cleanDest := filepath.Clean(newDest)
			return ioutil.WriteFile(cleanDest, data, 0600)
		}
		return nil
	})
	return err
}
