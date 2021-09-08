package publish

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/RedHatGov/bundle/pkg/cli"
	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/google/uuid"
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
	wantUUID, err := uuid.Parse("68a65604-fac7-4acf-98e1-eebaf59ddcb0")
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
				archivePath: "../../../test/publish/testdata/archives/testbundle_seq2.tar",
			},
			want:    &SequenceError{1, 2},
			wantErr: true,
		},
		{
			name:     "testing sequence out of order",
			metadata: "../../../test/publish/testdata/configs/one.json",
			fields: fields{
				archivePath: "../../../test/publish/testdata/archives/testbundle_seq3.tar",
			},
			want:    &SequenceError{2, 3},
			wantErr: true,
		},
		{
			name:     "testing uid mismatch",
			metadata: "../../../test/publish/testdata/configs/diff-uid.json",
			fields: fields{
				archivePath: "../../../test/publish/testdata/archives/testbundle_seq3.tar",
			},
			want:    &UuidError{wantUUID, gotUUID},
			wantErr: true,
		},
	}
	for _, tt := range tests {

		tmpdir := t.TempDir()

		opts := Options{
			RootOptions: &cli.RootOptions{
				IOStreams: genericclioptions.IOStreams{
					In:     os.Stdin,
					Out:    os.Stdout,
					ErrOut: os.Stderr,
				},
				Dir: tmpdir,
			},
			ArchivePath: tt.fields.archivePath,
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
			err = ioutil.WriteFile(filepath.Join(tmpdir, config.MetadataBasePath), input, 0644)
			if err != nil {
				t.Fatal(err)
			}
		}

		err := opts.Run(ctx, cmd, f)

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
