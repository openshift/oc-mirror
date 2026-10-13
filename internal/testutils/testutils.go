//nolint:wrapcheck // we do not care about wrapping errors in tests
package testutils

import (
	"bytes"
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
	"text/template"

	"github.com/distribution/distribution/v3/manifest"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/openshift/oc-mirror/v2/internal/pkg/image"
)

const (
	GraphMediaType = "application/json"
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
	_, err = buildAndPushFakeImage(c, targetRef, dir)
	if err != nil {
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
			return os.Mkdir(filepath.Join(destination, relPath), 0o750)
		default:
			newSource := filepath.Join(source, relPath)
			cleanSource := filepath.Clean(newSource)
			data, err := os.ReadFile(cleanSource)
			if err != nil {
				return err
			}
			newDest := filepath.Join(destination, relPath)
			cleanDest := filepath.Clean(newDest)
			return os.WriteFile(cleanDest, data, 0o600)
		}
		return nil
	})
	return err
}

func CreateRegistry() *httptest.Server {
	// Set up a fake registry.
	s := httptest.NewServer(registry.New())

	return s
}

func buildAndPushFakeImage(content map[string][]byte, imgRef string, dir string) (string, error) {
	var digest v1.Hash
	tag, err := name.NewTag(imgRef)
	if err != nil {
		return "", err
	}
	i, _ := crane.Image(content)
	if err := crane.Push(i, tag.String()); err != nil {
		return "", err
	}
	if dir == "" {
		digest, err = i.Digest()
		if err != nil {
			return "", err
		}
		return digest.String(), nil
	}
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
	digest, err = idx.Digest()
	if err != nil {
		return "", err
	}

	return digest.String(), nil
}

// GenerateFakeImage will use go-containerregistry to push a test image to
// an httptest.Server and will write the image to an OCI layout if dir is not "".
func GenerateFakeImage(content, imgRef string, tempFolder string) (string, error) {
	imgSpec, err := image.ParseRef(imgRef)
	if err != nil {
		return "", err
	}
	sanitizedTagOrDigest := imgSpec.Tag
	if imgSpec.Tag == "" {
		sanitizedTagOrDigest = imgSpec.Digest
	} else {
		sanitizedTagOrDigest = strings.ReplaceAll(sanitizedTagOrDigest, ".", "-")
	}

	indexFolder := filepath.Join(tempFolder, imgSpec.PathComponent, sanitizedTagOrDigest)
	err = os.MkdirAll(indexFolder, 0o755)
	if err != nil {
		return "", err
	}
	c := map[string][]byte{
		"/testfile": []byte("test contents " + content),
	}
	return buildAndPushFakeImage(c, imgRef, indexFolder)
}

func ByteArrayFromTemplate(templatePath string, tokens []string) ([]byte, error) {
	exBytes := []byte{}
	buf := bytes.NewBuffer(exBytes)
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return []byte{}, err
	}
	err = tmpl.Execute(buf, tokens)
	if err != nil {
		return []byte{}, err
	}
	return buf.Bytes(), nil
}

func ImgRefsFromTemplate(filePath, templatePath string, token releaseContents) error {
	exBytes := []byte{}
	buf := bytes.NewBuffer(exBytes)
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return err
	}
	err = tmpl.Execute(buf, token)
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, buf.Bytes(), 0o600)
}

func FileFromTemplate(filePath, templatePath string, tokens []string) error {
	bytes, err := ByteArrayFromTemplate(templatePath, tokens)
	if err != nil {
		return err
	}

	fmt.Printf("registries.conf contents: %v", string(bytes))
	return os.WriteFile(filePath, bytes, 0o600)
}

func ImageExists(imgRef string) (bool, error) {
	desc, err := crane.Head(imgRef)
	if err != nil {
		return false, err
	}
	if desc != nil {
		return true, nil
	}
	return false, nil
}

type releaseContents struct {
	Ref1 string
	Ref2 string
	Ref3 string
}

func GenerateReleaseAndComponents(toRegistry, tempFolder string, templatePath string) (string, []string, error) {
	contents := releaseContents{}
	relatedImages := []string{}
	// generating a fake release with 3 components
	component1 := toRegistry + "/openshift-release-dev/ocp-v4.0-art-dev:component1"
	digest1, err := GenerateFakeImage("component1", component1, tempFolder)
	if err != nil {
		return "", relatedImages, err
	}
	contents.Ref1 = toRegistry + "/openshift-release-dev/ocp-v4.0-art-dev@" + digest1
	relatedImages = append(relatedImages, contents.Ref1)

	component2 := toRegistry + "/openshift-release-dev/ocp-v4.0-art-dev:component2"
	digest2, err := GenerateFakeImage("component2", component2, tempFolder)
	if err != nil {
		return "", relatedImages, err
	}
	contents.Ref2 = toRegistry + "/openshift-release-dev/ocp-v4.0-art-dev@" + digest2
	relatedImages = append(relatedImages, contents.Ref2)

	component3 := toRegistry + "/openshift-release-dev/ocp-v4.0-art-dev:component3"
	digest3, err := GenerateFakeImage("component3", component3, tempFolder)
	if err != nil {
		return "", relatedImages, err
	}
	contents.Ref3 = toRegistry + "/openshift-release-dev/ocp-v4.0-art-dev@" + digest3
	relatedImages = append(relatedImages, contents.Ref3)

	digest, err := GenerateFakeRelease(contents, toRegistry+"/openshift-release-dev/ocp-release:4.15.0-x86_64", tempFolder, templatePath)
	if err != nil {
		return "", relatedImages, err
	}
	relatedImages = append(relatedImages, toRegistry+"/openshift-release-dev/ocp-release@"+digest)
	return digest, relatedImages, nil
}

func GenerateFakeRelease(imageRefs releaseContents, releaseImgRef, tempFolder string, templateFile string) (string, error) {
	err := ImgRefsFromTemplate(tempFolder+"/image-references", templateFile, imageRefs)
	if err != nil {
		return "", err
	}
	imgSpec, err := image.ParseRef(releaseImgRef)
	if err != nil {
		return "", err
	}
	sanitizedTagOrDigest := imgSpec.Tag
	if imgSpec.Tag == "" {
		sanitizedTagOrDigest = imgSpec.Digest
	} else {
		sanitizedTagOrDigest = strings.ReplaceAll(sanitizedTagOrDigest, ".", "-")
	}

	indexFolder := filepath.Join(tempFolder, imgSpec.PathComponent, sanitizedTagOrDigest)
	err = os.MkdirAll(indexFolder, 0o755)
	if err != nil {
		return "", err
	}

	imageRefsBytes, err := os.ReadFile(tempFolder + "/image-references")
	if err != nil {
		return "", err
	}
	releaseMetaBytes, err := os.ReadFile("../../e2e/templates/release_templates/release-metadata")
	if err != nil {
		return "", err
	}
	c := map[string][]byte{
		"/release-manifests/image-references": imageRefsBytes,
		"/release-manifests/release-metadata": releaseMetaBytes,
	}
	return buildAndPushFakeImage(c, releaseImgRef, indexFolder)
}

type CincinnatiMock struct {
	Templates map[string]string
	Tokens    []string
}

func (c CincinnatiMock) CincinnatiHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	mtype := r.Header.Get("Accept")
	if mtype != GraphMediaType {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}

	keys, ok := r.URL.Query()["channel"]
	if !ok {
		// t.Fail()
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ch := keys[len(keys)-1]

	responseTemplate, ok := c.Templates[ch]
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	responseBytes, err := ByteArrayFromTemplate(responseTemplate, c.Tokens)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, err = w.Write(responseBytes)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
