package publish

import (
	"context"
	"os"
	"testing"

	"github.com/RedHatGov/bundle/pkg/cli"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func Test_buildCatalogImage(t *testing.T) {

	ctx := context.Background()
	/*
		dctx := types.SystemContext{
			DockerInsecureSkipTLSVerify: types.NewOptionalBool(true),
		}
	*/
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
				Dir:     tmpdir,
				SkipTLS: true,
			},
			ArchivePath: tt.fields.archivePath,
			CatalogPlatforms: []string{
				"linux/amd64",
				"linux/arm64",
			},
		}

		ref := reference.DockerImageReference{
			Registry:  "localhost:5000",
			Namespace: "test",
			Name:      "testname",
			Tag:       "vtest3",
		}

		dir := t.TempDir()

		err := opts.buildCatalogImage(ctx, ref, dir)
		require.NoError(t, err)
		t.Log(err)
	}

}
