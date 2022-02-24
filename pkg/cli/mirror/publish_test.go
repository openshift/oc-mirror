package mirror

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/uuid"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
)

func TestMetadataError(t *testing.T) {

	ctx := context.Background()

	// Set up expected UUIDs
	gotUUID, err := uuid.Parse("360a43c2-8a14-4b5d-906b-07491459f25f")
	require.NoError(t, err)

	type fields struct {
		archivePath string
	}
	tests := []struct {
		name     string
		metadata string
		fields   fields
		want     error
		wantErr  bool
	}{
		{
			name:     "Invalid/FirstRun",
			metadata: "",
			fields: fields{
				archivePath: "testdata/artifacts/testbundle_seq2.tar",
			},
			want:    &SequenceError{1, 2},
			wantErr: true,
		},
		{
			name:     "Invalid/OutOfOrder",
			metadata: "testdata/configs/one.json",
			fields: fields{
				archivePath: "testdata/artifacts/testbundle_seq3.tar",
			},
			want:    &SequenceError{2, 3},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(registry.New())
			t.Cleanup(server.Close)
			u, err := url.Parse(server.URL)
			require.NoError(t, err)

			tmpdir := t.TempDir()

			opts := &MirrorOptions{
				RootOptions: &cli.RootOptions{
					IOStreams: genericclioptions.IOStreams{
						In:     os.Stdin,
						Out:    os.Stdout,
						ErrOut: os.Stderr,
					},
					Dir: tmpdir,
				},
				DestSkipTLS: true,
				From:        tt.fields.archivePath,
				ToMirror:    u.Host,
			}

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

			_, err = opts.Publish(ctx)

			if !tt.wantErr {
				require.NoError(t, err)
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
				Namespace: "foo",
				Name:      "test3/baz",
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

			meta := v1alpha2.Metadata{
				MetadataSpec: v1alpha2.MetadataSpec{
					PastBlobs: []v1alpha2.Blob{
						{
							ID:            "found",
							NamespaceName: "test1/baz",
							TimeStamp:     1644586136,
						},
						{
							ID:            "found",
							NamespaceName: "test2/baz",
							TimeStamp:     1644586137,
						},
						{
							ID:            "found",
							NamespaceName: "test3/baz",
							TimeStamp:     1644586139,
						},
					},
				},
			}

			sort.Sort(sort.Reverse(meta.PastBlobs))

			ref, err := test.options.findBlobRepo(meta.PastBlobs, test.digest)
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
