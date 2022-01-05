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
	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func Test_MetadataError(t *testing.T) {

	cmd := &cobra.Command{}
	ctx := context.Background()

	// Configures a REST client getter factory from configs for mirroring releases.
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDiscoveryBurst(250)
	matchVersionKubeConfigFlags := kcmdutil.NewMatchVersionFlags(kubeConfigFlags)

	f := kcmdutil.NewFactory(matchVersionKubeConfigFlags)

	// Set up expected UUIDs
	gotUUID, err := uuid.Parse("360a43c2-8a14-4b5d-906b-07491459f25f")
	if err != nil {
		t.Fatal(err)
	}

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
			name:     "testing first metadata error",
			metadata: "",
			fields: fields{
				archivePath: "testdata/artifacts/testbundle_seq2.tar",
			},
			want:    &SequenceError{1, 2},
			wantErr: true,
		},
		{
			name:     "testing sequence out of order",
			metadata: "testdata/configs/one.json",
			fields: fields{
				archivePath: "testdata/artifacts/testbundle_seq3.tar",
			},
			want:    &SequenceError{2, 3},
			wantErr: true,
		},
	}
	for _, tt := range tests {

		server := httptest.NewServer(registry.New())
		t.Cleanup(server.Close)
		u, err := url.Parse(server.URL)
		if err != nil {
			t.Error(err)
			continue
		}

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
			if err != nil {
				t.Fatal(err)
			}

			if err := os.Mkdir(filepath.Join(tmpdir, config.PublishDir), os.ModePerm); err != nil {
				t.Fatal(err)
			}
			if err := ioutil.WriteFile(filepath.Join(tmpdir, config.MetadataBasePath), input, 0644); err != nil {
				t.Fatal(err)
			}

			if err := prepMetadata(ctx, u.Host, tmpdir, gotUUID.String()); err != nil {
				t.Fatal(err)
			}
		}

		err = opts.Publish(ctx, cmd, f)

		if !tt.wantErr {
			if err != nil {
				t.Errorf("Test %s error received when checking metadata: %v", tt.name, err)
			}
		} else {
			if err.Error() != tt.want.Error() {
				t.Errorf("Test %s wrong error type. Want \"%v\", got \"%v\"", tt.name, tt.want, err)
			}
		}
	}
}

// prepareMetadata will ensure metdata is in the registry for testing
func prepMetadata(ctx context.Context, host, dir, uuid string) error {
	var meta v1alpha1.Metadata

	cfg := v1alpha1.StorageConfig{
		Registry: &v1alpha1.RegistryConfig{
			ImageURL: fmt.Sprintf("%s/oc-mirror:%s", host, uuid),
			SkipTLS:  true,
		},
	}
	registry, err := storage.ByConfig(dir, cfg)
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

	return registry.WriteMetadata(ctx, &meta, dir)
}
