package mirror

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/require"
)

func TestGetImageDigests(t *testing.T) {
	// function to calculate a sha from provided data
	sha256String := func(data []byte) string {
		h := sha256.Sum256(data)
		return fmt.Sprintf("sha256:%s", hex.EncodeToString(h[:]))
	}

	type test struct {
		name                  string          // name of test
		manifestResponse      []byte          // response data
		configResponse        []byte          // config file response
		contentType           types.MediaType // content type for the test
		tagOrDigest           string          // tag or digest for the test
		configDigest          string          //
		repo                  string          // the repo for the test
		expectedDigests       []string        // the expected digest values for the test
		expectedArchitectures int             // number of expected architectures
	}

	tests := []test{
		{
			name: "single image",
			manifestResponse: []byte(`{
    "schemaVersion": 2,
    "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
    "config": {
        "mediaType": "application/vnd.docker.container.image.v1+json",
        "size": 3710,
        "digest": "sha256:c2dd448de7df351271662c4bac7e797426c877b398b43c51aa2d9fb6be73c30d"
    },
    "layers": [
        {
            "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
            "size": 824742,
            "digest": "sha256:fb65518b46d2886c4a083a5efdc63cd3a85ebb5134e91302075a588a93e2ae6d"
        },
        {
            "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
            "size": 760242,
            "digest": "sha256:dd9f516e061e2929ffad5aaaa9baf2b0bcb900fa17cd24f670f090c78687a961"
        },
        {
            "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
            "size": 3891539,
            "digest": "sha256:09eb306efc9c1b1d983053ffe831817aa0a204e00516a89b83c3ad2a2a90bd8b"
        },
        {
            "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
            "size": 165,
            "digest": "sha256:73a36c7de5f87901fac92f94e963356d05144154352529922220c4b1ef4df88f"
        },
        {
            "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
            "size": 15936019,
            "digest": "sha256:444082a9d028ace84bd238515264c654de9364f56479c675f74765c8a4d81229"
        },
        {
            "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
            "size": 18831,
            "digest": "sha256:36fb4d523c39d524251be457b75e00bcfd793025e8de76ca4c938ea65f4a9056"
        }
    ]
}`),
			configResponse: []byte(`{
    "created": "2022-12-22T20:29:07.771360817Z",
    "author": "Bazel",
    "architecture": "s390x",
    "os": "linux",
    "config": {
        "User": "1001",
        "Env": [
            "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/busybox",
            "SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt"
        ],
        "Entrypoint": [
            "/bin/opm"
        ],
        "Cmd": [
            "serve",
            "/configs"
        ],
        "WorkingDir": "/",
        "Labels": {
            "io.buildah.version": "1.28.0",
            "operators.operatorframework.io.index.configs.v1": "/configs"
        }
    },
    "rootfs": {
        "type": "layers",
        "diff_ids": [
            "sha256:1d71208a40b398f57344564f6e13d1212d43bcaee898e16f9b2d7b79eb51ca8c",
            "sha256:b441fdb3f7d10892c570ab76731f0970bc28b2c3d3a9057a8d7dbf4662009edc",
            "sha256:8aa7344792c2813367c0330703bc3b3efddb9e9bf3c1d5586940441551f3502d",
            "sha256:cc843c8d783852eb188440611819378697cd4635a51b22dba22d3c393b29601a",
            "sha256:05a264babaf1644870383690ca9e82cdf40d8d3622ee4f8250df42d0c63a3a9b",
            "sha256:9e4f0da3b9aab2f31400159a4b6377bbbb0df9d751287344d3885f3ef1ffda08"
        ]
    },
    "history": [
        {
            "created": "1970-01-01T00:00:00Z",
            "created_by": "bazel build ...",
            "author": "Bazel"
        },
        {
            "created": "1970-01-01T00:00:00Z",
            "created_by": "bazel build ...",
            "author": "Bazel"
        },
        {
            "created": "2022-09-21T15:35:14.452625838Z",
            "created_by": "COPY /ko-app/grpc-health-probe /bin/grpc_health_probe # buildkit",
            "comment": "buildkit.dockerfile.v0"
        },
        {
            "created": "2022-09-21T15:35:14.49830882Z",
            "created_by": "COPY nsswitch.conf /etc/nsswitch.conf # buildkit",
            "comment": "buildkit.dockerfile.v0"
        },
        {
            "created": "2022-09-21T15:35:14.776295779Z",
            "created_by": "COPY opm /bin/opm # buildkit",
            "comment": "buildkit.dockerfile.v0"
        },
        {
            "created": "2022-09-21T15:35:14.776295779Z",
            "created_by": "USER 1001",
            "comment": "buildkit.dockerfile.v0",
            "empty_layer": true
        },
        {
            "created": "2022-09-21T15:35:14.776295779Z",
            "created_by": "ENTRYPOINT [\"/bin/opm\"]",
            "comment": "buildkit.dockerfile.v0",
            "empty_layer": true
        },
        {
            "created": "2022-12-22T20:29:07.231373313Z",
            "created_by": "/bin/sh -c #(nop) ENTRYPOINT [\"/bin/opm\"]",
            "author": "Bazel",
            "comment": "FROM quay.io/operator-framework/opm:latest",
            "empty_layer": true
        },
        {
            "created": "2022-12-22T20:29:07.317353099Z",
            "created_by": "/bin/sh -c #(nop) CMD [\"serve\", \"/configs\"]",
            "author": "Bazel",
            "comment": "FROM 8e8d3b76eca5",
            "empty_layer": true
        },
        {
            "created": "2022-12-22T20:29:07.638816637Z",
            "created_by": "/bin/sh -c #(nop) ADD dir:eb23b1db6b40cb6d0a88b01dae60474fdbd0eac0a9f1eaa04274949884b2c026 in /configs ",
            "author": "Bazel",
            "comment": "FROM 7f535ae3e459"
        },
        {
            "created": "2022-12-22T20:29:07.771663071Z",
            "created_by": "/bin/sh -c #(nop) LABEL operators.operatorframework.io.index.configs.v1=/configs",
            "author": "Bazel",
            "comment": "FROM 1db572c343ce",
            "empty_layer": true
        }
    ]
}`),
			configDigest: "sha256:c2dd448de7df351271662c4bac7e797426c877b398b43c51aa2d9fb6be73c30d",
			contentType:  types.DockerManifestSchema2,
			tagOrDigest:  "latest",
			repo:         "foo/bar",
			expectedDigests: []string{
				"sha256:019dce3f90770a275e560a80eacbcd5768c34d80b5bd127b461ed710636c709a",
			},
			expectedArchitectures: 1,
		},
		{
			name: "manifest list image with tag",
			manifestResponse: []byte(`{
    "schemaVersion": 2,
    "mediaType": "application/vnd.docker.distribution.manifest.list.v2+json",
    "manifests": [
        {
            "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
            "size": 1241,
            "digest": "sha256:cf38d82d48a81f1f7363faa59b80f181efa81832f831529d0e239f7ea6223047",
            "platform": {
                "architecture": "s390x",
                "os": "linux"
            }
        },
        {
            "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
            "size": 1241,
            "digest": "sha256:5d961dbf9ab449b164685ea30e7c70f986b9595370c98f8712d660250b5c386b",
            "platform": {
                "architecture": "amd64",
                "os": "linux"
            }
        }
    ]
}`),
			contentType: types.DockerManifestList,
			tagOrDigest: "latest",
			repo:        "foo/bar",
			expectedDigests: []string{
				"sha256:cf38d82d48a81f1f7363faa59b80f181efa81832f831529d0e239f7ea6223047",
				"sha256:5d961dbf9ab449b164685ea30e7c70f986b9595370c98f8712d660250b5c386b",
			},
			expectedArchitectures: 2,
		},
		{
			name: "manifest list image with digest",
			manifestResponse: []byte(`{
    "schemaVersion": 2,
    "mediaType": "application/vnd.docker.distribution.manifest.list.v2+json",
    "manifests": [
        {
            "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
            "size": 1241,
            "digest": "sha256:cf38d82d48a81f1f7363faa59b80f181efa81832f831529d0e239f7ea6223047",
            "platform": {
                "architecture": "s390x",
                "os": "linux"
            }
        },
        {
            "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
            "size": 1241,
            "digest": "sha256:5d961dbf9ab449b164685ea30e7c70f986b9595370c98f8712d660250b5c386b",
            "platform": {
                "architecture": "amd64",
                "os": "linux"
            }
        }
    ]
}`),
			contentType: types.DockerManifestList,
			tagOrDigest: "sha256:b4ee287b254c3254a9a8e8b4fb2244c53db1590e7b6c6cc069b90e24af7f759f",
			repo:        "foo/bar",
			expectedDigests: []string{
				"sha256:cf38d82d48a81f1f7363faa59b80f181efa81832f831529d0e239f7ea6223047",
				"sha256:5d961dbf9ab449b164685ea30e7c70f986b9595370c98f8712d660250b5c386b",
			},
			expectedArchitectures: 2,
		},
	}
	// ddd := sha256String(tests[0].manifestResponse)
	// _ = ddd
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// setup and run a http server to simulate a docker registry

			// manifest path represents the request for a manifest document
			manifestPath := fmt.Sprintf("/v2/%s/manifests/%s", test.repo, test.tagOrDigest)
			// config path represents the request for a config file document
			configPath := fmt.Sprintf("/v2/%s/blobs/%s", test.repo, test.configDigest)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v2/":
					w.WriteHeader(http.StatusOK)
				case manifestPath:
					w.Header().Set("Content-Type", string(test.contentType))
					w.Header().Set("Docker-Content-Digest", sha256String(test.manifestResponse))
					switch r.Method {
					case http.MethodGet:
						w.Write(test.manifestResponse)
					case http.MethodHead:
						w.Header().Set("Content-Length", "0") // head requests don't send a body
					}
				case configPath:
					w.Header().Set("Content-Type", string(test.contentType))
					w.Header().Set("Docker-Content-Digest", sha256String(test.manifestResponse))
					w.Write(test.configResponse)
				default:
					t.Fatalf("Unexpected path: %v", r.URL.Path)
				}
			}))
			defer server.Close()

			url, err := url.Parse(server.URL)
			require.NoError(t, err)

			symbol := ":"
			if strings.HasPrefix(test.tagOrDigest, "sha256:") {
				symbol = "@"
			}

			// call function to test
			ref := fmt.Sprintf("%s/%s%s%s", url.Host, test.repo, symbol, test.tagOrDigest)
			digestsMap, err := getImageDigests(context.TODO(), ref, nil, false)
			require.NoError(t, err)
			require.Len(t, digestsMap, test.expectedArchitectures)

			// create expected / actual values in common format... lump all of the architectures together in this slice
			actualDigestsAsString := []string{}
			for platform, catalogMetadata := range digestsMap {
				// we should have a platform value that's not a "zero" value
				require.NotZero(t, platform)

				actualDigestsAsString = append(actualDigestsAsString, catalogMetadata.catalogRef.Name())

			}

			expectedDigestsAsString := []string{}
			for _, expectedDigest := range test.expectedDigests {
				expectedDigestsAsString = append(expectedDigestsAsString, fmt.Sprintf("%s/%s@%s", url.Host, test.repo, expectedDigest))
			}

			// check expected / actual values
			require.Equal(t, expectedDigestsAsString, actualDigestsAsString)
		})
	}
}

func TestGetImageDigestsFromOCILayout(t *testing.T) {

	type test struct {
		name                  string // name of test
		imageRef              string
		layoutPath            layout.Path
		expectedArchitectures int      // number of expected architectures
		expectedDigests       []string // the expected digest values for the test
	}

	tests := []test{
		{
			name:                  "manifest list",
			imageRef:              "index.docker.io/library/hello-world",
			layoutPath:            layout.Path("testdata/manifestlist/hello"),
			expectedArchitectures: 2,
			expectedDigests: []string{
				"sha256:f54a58bc1aac5ea1a25d796ae155dc228b3f0e11d046ae276b39c4bf2f13d8c4",
				"sha256:c7b6944911848ce39b44ed660d95fb54d69bbd531de724c7ce6fc9f743c0b861",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			digestsMap, err := getImageDigests(context.TODO(), test.imageRef, &test.layoutPath, false)
			require.NoError(t, err)
			require.Len(t, digestsMap, test.expectedArchitectures)
			// create expected / actual values in common format... lump all of the architectures together in this slice
			actualDigestsAsString := []string{}
			for platform, catalogMetadata := range digestsMap {
				// we should have a platform value that's not a "zero" value
				require.NotZero(t, platform)

				actualDigestsAsString = append(actualDigestsAsString, catalogMetadata.catalogRef.Name())
			}

			expectedDigestsAsString := []string{}
			for _, expectedDigest := range test.expectedDigests {
				expectedDigestsAsString = append(expectedDigestsAsString, fmt.Sprintf("%s@%s", test.imageRef, expectedDigest))
			}

			// check expected / actual values (order does not matter)
			require.ElementsMatch(t, expectedDigestsAsString, actualDigestsAsString)
		})
	}
}

func TestGetDigestFromOCILayout(t *testing.T) {

	type test struct {
		name         string // name of test
		layoutPath   layout.Path
		platformIn   OperatorCatalogPlatform
		expectedHash *v1.Hash
	}

	tests := []test{
		{
			name:       "manifest list - amd64",
			layoutPath: layout.Path("testdata/manifestlist/hello"),
			platformIn: OperatorCatalogPlatform{
				os:           "linux",
				architecture: "amd64",
				isIndex:      true,
			},
			expectedHash: &v1.Hash{
				Algorithm: "sha256",
				Hex:       "f54a58bc1aac5ea1a25d796ae155dc228b3f0e11d046ae276b39c4bf2f13d8c4",
			},
		},
		{
			name:       "manifest list - s390x",
			layoutPath: layout.Path("testdata/manifestlist/hello"),
			platformIn: OperatorCatalogPlatform{
				os:           "linux",
				architecture: "s390x",
				isIndex:      true,
			},
			expectedHash: &v1.Hash{
				Algorithm: "sha256",
				Hex:       "c7b6944911848ce39b44ed660d95fb54d69bbd531de724c7ce6fc9f743c0b861",
			},
		},
		{
			name:       "single arch",
			layoutPath: layout.Path("testdata/artifacts/rhop-ctlg-oci"),
			platformIn: OperatorCatalogPlatform{
				isIndex: false,
			},
			expectedHash: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hash, err := getDigestFromOCILayout(context.TODO(), test.layoutPath, test.platformIn)
			require.NoError(t, err)
			require.Equal(t, test.expectedHash, hash)
		})
	}
}
