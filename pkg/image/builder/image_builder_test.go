package builder

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/match"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/internal/testutils"
)

func TestCreateLayout(t *testing.T) {

	tests := []struct {
		name          string
		existingImage bool
		err           string
	}{
		{
			name:          "Success/ExistingImage",
			existingImage: true,
		},
		{
			name:          "Success/NewImage",
			existingImage: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			tmpdir := t.TempDir()
			server := httptest.NewServer(registry.New())
			t.Cleanup(server.Close)

			targetRef, err := testutils.WriteTestImage(server, tmpdir)
			require.NoError(t, err)

			builder := NewImageBuilder([]name.Option{name.Insecure}, nil)

			var lp layout.Path
			if test.existingImage {
				lp, err = builder.CreateLayout(targetRef, t.TempDir())
			} else {
				lp, err = builder.CreateLayout("", tmpdir)
			}

			if test.err == "" {
				require.NoError(t, err)
				ii, err := lp.ImageIndex()
				require.NoError(t, err)
				im, err := ii.IndexManifest()
				require.NoError(t, err)
				require.Len(t, im.Manifests, 1)
			} else {
				require.EqualError(t, err, test.err)
			}
		})
	}
}

func TestRun(t *testing.T) {

	tests := []struct {
		name             string
		existingImage    bool
		pinToDigest      bool
		update           configUpdateFunc
		configAssertFunc func(cfg v1.ConfigFile) bool
		multiarch        bool // create a multi arch image
		matcher          match.Matcher
		err              error
	}{
		{
			name:          "Success/ExistingImage",
			existingImage: true,
		},
		{
			name:          "Success/NewImage",
			existingImage: false,
		},
		{
			name:          "Success/WithConfigUpdate",
			existingImage: true,
			update: func(cfg *v1.ConfigFile) {
				cfg.Config.Cmd = []string{"newcommand"}
			},
			configAssertFunc: func(cfg v1.ConfigFile) bool {
				return cfg.Config.Cmd[0] == "newcommand"
			},
		},
		{
			name:        "Failure/DigestReference",
			pinToDigest: true,
			err:         &ErrInvalidReference{},
		},
		{
			name:          "Success/ExistingImage - multi arch",
			existingImage: true,
			multiarch:     true,
		},
		{
			name:          "Success/NewImage - multi arch",
			existingImage: false,
			multiarch:     true,
		},
		{
			name:          "Success/WithConfigUpdate - multi arch",
			existingImage: true,
			update: func(cfg *v1.ConfigFile) {
				cfg.Config.Cmd = []string{"newcommand"}
			},
			configAssertFunc: func(cfg v1.ConfigFile) bool {
				return cfg.Config.Cmd[0] == "newcommand"
			},
			multiarch: true,
		},
		{
			name:        "Failure/DigestReference - multi arch",
			pinToDigest: true,
			err:         &ErrInvalidReference{},
			multiarch:   true,
		},
		{
			name:          "Success/ExistingImage - multi arch with filter",
			existingImage: true,
			multiarch:     true,
			// we'd like to test with digests here, but the test dynamically creates images so use platform instead
			matcher: match.Platforms(v1.Platform{OS: "linux", Architecture: "ppc64le"}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			tmpdir := t.TempDir()

			// each test case gets its own server
			server := httptest.NewServer(registry.New())
			t.Cleanup(server.Close)

			var targetRef string
			var err error
			if test.multiarch {
				targetRef, err = testutils.WriteMultiArchTestImage(server, tmpdir)
				// targetRef, err := testutils.WriteMultiArchTestImageWithURL("http://localhost:5000", tmpdir)
			} else {
				targetRef, err = testutils.WriteTestImage(server, tmpdir)
				// targetRef, err := testutils.WriteTestImageWithURL("http://localhost:5000", tmpdir)
			}

			require.NoError(t, err)

			if test.pinToDigest {
				targetRef, err = pinToDigest(targetRef)
				require.NoError(t, err)
			}

			d1 := []byte("hello\ngo\n")
			require.NoError(t, os.WriteFile(filepath.Join(tmpdir, "test"), d1, 0644))

			add, err := LayerFromPath("/testfile", filepath.Join(tmpdir, "test"))
			require.NoError(t, err)

			builder := NewImageBuilder([]name.Option{name.Insecure}, nil)

			var layout layout.Path
			if test.existingImage {
				layout, err = builder.CreateLayout(targetRef, t.TempDir())
				require.NoError(t, err)
			} else {
				layout, err = builder.CreateLayout("", tmpdir)
				require.NoError(t, err)
			}
			err = builder.Run(context.Background(), targetRef, layout, test.matcher, test.update, []v1.Layer{add}...)
			if test.err == nil {
				require.NoError(t, err)

				// Get new image information
				ref, err := name.ParseReference(targetRef, name.Insecure)
				require.NoError(t, err)

				/*
					There's an important distinction between what's possible in OCI layout versus docker registries,
					and its important to understand when reading this test. The WriteMultiArchTestImage function
					creates an OCI layout AND pushes to the dummy registry.

					An OCI layout path contains an index.json where the manifest entries reference either a manifest list or an actual image.
					Since the index.json is technically its own index, you can have "index indirection" where the manifest entry in index.json
					points to a SHA within the blobs directory that is itself an "index". For example:

					Single arch in an index.json
					{
						"schemaVersion": 2,
						"manifests": [
							{
								"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
								"digest": "sha256:1234...",
								"size": 111
							}
						]
					}
					multi arch with "indirection" in an index.json
					{
						"schemaVersion": 2,
						"manifests": [
							{
								"mediaType": "application/vnd.docker.distribution.manifest.list.v2+json",
								"digest": "sha256:5678...",
								"size": 321
							}
						]
					}

					multi arch without "indirection" in an index.json (this scenario is probably not likely)
					{
					   "schemaVersion": 2,
					   "mediaType": "application/vnd.docker.distribution.manifest.list.v2+json",
					   "manifests": [
					      {
					         "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
					         "size": 525,
					         "digest": "sha256:9123...",
					         "platform": {
					            "architecture": "amd64",
					            "os": "linux"
					         }
					      },
					      {
					         "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
					         "size": 525,
					         "digest": "sha256:4567...",
					         "platform": {
					            "architecture": "s390x",
					            "os": "linux"
					         }
					      }
					   ]
					}

					However, in a "remote" docker registry, manifest indirection does not exist. So when reading
					the descriptor from the "remote", we should expect to see a direct reference to either the
					manifest list or image.
				*/
				var desc *remote.Descriptor
				// make remote call to our dummy server to get its descriptor
				desc, err = remote.Get(ref)
				require.NoError(t, err)
				// figure out an image to test against
				var img v1.Image

				if test.multiarch {
					require.True(t, desc.MediaType.IsIndex(), "expected a multi arch index")

					idx, err := desc.ImageIndex()
					require.NoError(t, err)
					mf, err := idx.IndexManifest()
					require.NoError(t, err)

					// during iteration we'll either find a matching image (if a matcher is used in the test)
					// or we'll just return the last image we find
					for _, innerDescriptor := range mf.Manifests {
						if test.matcher == nil || (test.matcher != nil && test.matcher(innerDescriptor)) {
							img, err = idx.Image(innerDescriptor.Digest)
							require.NoError(t, err)
						}
					}
				} else {
					// single arch tests we can just get the image directly
					// NOTE: WriteTestImage pushes an OCI image to the registry, so its technically a "manifest list"
					// and the image is "resolved" using the platform associated with the image reference, or the default
					// (i.e. linux/amd64) if platform is not present
					img, err = desc.Image()
					require.NoError(t, err)
				}

				// make sure we've actually found an image to work with
				require.NotNil(t, img)
				layers, err := img.Layers()
				require.NoError(t, err)
				idx, err := desc.ImageIndex()
				require.NoError(t, err)
				im, err := idx.IndexManifest()
				require.NoError(t, err)

				// Check that new layer is present
				expectedDigest, err := add.Digest()
				require.NoError(t, err)
				var found bool
				for _, ly := range layers {
					dg, err := ly.Digest()
					require.NoError(t, err)
					if dg == expectedDigest {
						found = true
					}
				}
				require.True(t, found)
				if test.multiarch {
					// multi arch test has two manifests (linux/amd64 and linux/s390x)
					require.Len(t, im.Manifests, 2)
				} else {
					// single arch test has a single image, so only one manifest
					require.Len(t, im.Manifests, 1)
				}

				if test.update != nil {
					config, err := img.ConfigFile()
					require.NoError(t, err)
					require.True(t, test.configAssertFunc(*config))
				}

			} else {
				require.ErrorAs(t, err, &test.err)
			}
		})
	}
}

func NewTestServerWithURL(URL string, handler http.Handler) (*httptest.Server, error) {
	ts := httptest.NewUnstartedServer(handler)

	if URL != "" {
		l, err := net.Listen("tcp", URL)
		if err != nil {
			return nil, err
		}
		ts.Listener.Close()
		ts.Listener = l
	}
	ts.Start()
	return ts, nil
}

func TestLayoutFromPath(t *testing.T) {

	tests := []struct {
		name       string
		dir        bool
		targetPath string
		err        string
	}{
		{
			name:       "Valid/DirPath",
			targetPath: "testdir/",
			dir:        true,
		},
		{
			name:       "Valid/FilePath",
			targetPath: "testfile",
			dir:        false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			tmpdir := t.TempDir()

			// prep directory with files to write into layer
			d1 := []byte("hello\ngo\n")
			require.NoError(t, os.WriteFile(filepath.Join(tmpdir, "test"), d1, 0644))

			var sourcePath string
			if test.dir {
				sourcePath = tmpdir
			} else {
				sourcePath = filepath.Join(tmpdir, "test")
			}

			layer, err := LayerFromPath(test.targetPath, sourcePath)
			if test.err == "" {
				require.NoError(t, err)
				digest, err := layer.Digest()
				require.NoError(t, err)
				require.Contains(t, digest.String(), ":")
			} else {
				require.EqualError(t, err, test.err)
			}
		})
	}
}

func pinToDigest(unpinnedImage string) (string, error) {
	ref, err := reference.Parse(unpinnedImage)
	if err != nil {
		return "", err
	}
	ref.ID = "sha256:fc1ca63b4a6ac038808ae33c4498b122f9cf7a43dca278228e985986d3f81091"
	return ref.Exact(), nil
}
