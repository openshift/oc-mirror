package mirror

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/uuid"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
)

func TestHandleMetadata(t *testing.T) {

	ctx := context.Background()

	// Set up expected UUIDs
	gotUUID, err := uuid.Parse("360a43c2-8a14-4b5d-906b-07491459f25f")
	require.NoError(t, err)

	type spec struct {
		name        string
		opts        *MirrorOptions
		metadata    string
		expCurr     int
		expIncoming int
		want        error
		wantErr     bool
	}

	tests := []spec{
		{
			name: "Invalid/FirstRun",

			metadata: "",
			opts: &MirrorOptions{
				RootOptions: &cli.RootOptions{
					IOStreams: genericclioptions.IOStreams{
						In:     os.Stdin,
						Out:    os.Stdout,
						ErrOut: os.Stderr,
					},
				},
				DestSkipTLS: true,
				From:        "testdata/artifacts/testbundle_seq2.tar",
			},
			want:    &ErrInvalidSequence{1, 2},
			wantErr: true,
		},
		{
			name:     "Invalid/OutOfOrder",
			metadata: "testdata/configs/one.json",
			opts: &MirrorOptions{
				RootOptions: &cli.RootOptions{
					IOStreams: genericclioptions.IOStreams{
						In:     os.Stdin,
						Out:    os.Stdout,
						ErrOut: os.Stderr,
					},
				},
				DestSkipTLS: true,
				From:        "testdata/artifacts/testbundle_seq3.tar",
			},
			want:    &ErrInvalidSequence{2, 3},
			wantErr: true,
		},
		{
			name:        "Valid/SkipMetadataCheck",
			metadata:    "testdata/configs/one.json",
			expCurr:     1,
			expIncoming: 3,
			opts: &MirrorOptions{
				RootOptions: &cli.RootOptions{
					IOStreams: genericclioptions.IOStreams{
						In:     os.Stdin,
						Out:    os.Stdout,
						ErrOut: os.Stderr,
					},
				},
				DestSkipTLS:       true,
				SkipMetadataCheck: true,
				From:              "testdata/artifacts/testbundle_seq3.tar",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			tmpdir := t.TempDir()
			server := httptest.NewServer(registry.New())
			t.Cleanup(server.Close)
			u, err := url.Parse(server.URL)
			require.NoError(t, err)
			tt.opts.Dir = tmpdir
			tt.opts.ToMirror = u.Host

			// Copy metadata in place for tests with existing
			if tt.metadata != "" {
				t.Log("Copying metadata")
				input, err := ioutil.ReadFile(tt.metadata)
				require.NoError(t, err)
				err = os.MkdirAll(filepath.Join(tmpdir, config.PublishDir), os.ModePerm)
				require.NoError(t, err)
				err = ioutil.WriteFile(filepath.Join(tmpdir, config.MetadataBasePath), input, 0644)
				require.NoError(t, err)
				err = prepMetadata(ctx, u.Host, tmpdir, gotUUID.String())
				require.NoError(t, err)
			}

			filesInArchive := map[string]string{config.MetadataBasePath: tt.opts.From}

			_, incoming, curr, err := tt.opts.handleMetadata(ctx, filepath.Join(tmpdir, "foo"), filesInArchive)

			if !tt.wantErr {
				require.NoError(t, err)
				require.Equal(t, tt.expIncoming, incoming.PastMirror.Sequence)
				require.Equal(t, tt.expCurr, curr.PastMirror.Sequence)
			} else {
				require.EqualError(t, err, tt.want.Error())
			}
		})
	}
}

func TestFindBlobRepo(t *testing.T) {
	tests := []struct {
		name string

		digest   string
		meta     v1alpha2.Metadata
		options  *MirrorOptions
		expected imagesource.TypedImageReference
		err      string
	}{{
		name:   "Valid/NoUserNamespace",
		digest: "found",
		options: &MirrorOptions{
			ToMirror: "registry.com",
		},
		expected: imagesource.TypedImageReference{
			Type: "docker",
			Ref: reference.DockerImageReference{
				Registry:  "registry.com",
				Namespace: "test3",
				Name:      "baz",
			},
		},
	}, {
		name:   "Valid/UserNamespaceAdded",
		digest: "found",
		options: &MirrorOptions{
			ToMirror:      "registry.com",
			UserNamespace: "foo",
		},
		expected: imagesource.TypedImageReference{
			Type: "docker",
			Ref: reference.DockerImageReference{
				Registry:  "registry.com",
				Namespace: "foo/test3",
				Name:      "baz",
			},
		},
	}, {
		name:   "Invalid/NoRefExisting",
		digest: "notfound",
		options: &MirrorOptions{
			ToMirror: "registry.com",
		},
		err: "layer \"notfound\" is not present in previous metadata",
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			assocs := image.AssociationSet{"registry.com/test3/baz": image.Associations{
				"registry.com/test3/baz": {
					Name:            "registry.com/test3/baz",
					Path:            "single_manifest",
					TagSymlink:      "latest",
					ID:              "",
					Type:            v1alpha2.TypeGeneric,
					ManifestDigests: nil,
					LayerDigests:    []string{"found"},
				},
			},
			}

			ref, err := test.options.findBlobRepo(assocs, test.digest)
			if len(test.err) != 0 {
				require.Equal(t, err.Error(), test.err)
			} else {
				require.Equal(t, test.expected, ref)
			}
		})
	}
}

// prepareMetadata will ensure metadata is in the registry for testing
func prepMetadata(ctx context.Context, host, dir, uuid string) error {
	var meta v1alpha2.Metadata

	cfg := v1alpha2.StorageConfig{
		Registry: &v1alpha2.RegistryConfig{
			ImageURL: fmt.Sprintf("%s/oc-mirror:%s", host, uuid),
			SkipTLS:  true,
		},
	}

	reg, err := storage.ByConfig(dir, cfg)
	if err != nil {
		return err
	}
	local, err := storage.NewLocalBackend(dir)
	if err != nil {
		return err
	}

	if err := local.ReadMetadata(ctx, &meta, config.MetadataBasePath); err != nil {
		return err
	}

	return reg.WriteMetadata(ctx, &meta, dir)
}
