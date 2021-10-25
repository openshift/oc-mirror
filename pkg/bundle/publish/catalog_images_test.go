package publish

/* TODO: add test after podman/buildx is gone
import (
	"context"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/RedHatGov/bundle/pkg/cli"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func Test_buildCatalogImage(t *testing.T) {

	s := httptest.NewServer(registry.New())
	defer s.Close()

	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
		dctx := types.SystemContext{
			DockerInsecureSkipTLSVerify: types.NewOptionalBool(true),
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
	}
	for _, tt := range tests {

		dcDir := t.TempDir()
		dockerfileDir := t.TempDir()

		opts := Options{
			RootOptions: &cli.RootOptions{
				IOStreams: genericclioptions.IOStreams{
					In:     os.Stdin,
					Out:    os.Stdout,
					ErrOut: os.Stderr,
				},
				Dir:     ".",
				SkipTLS: true,
			},
			ArchivePath: tt.fields.archivePath,
		}

		ref := reference.DockerImageReference{
			Registry:  u.Host,
			Namespace: "test",
			Name:      "testname",
			Tag:       "vtest3",
		}

		err := opts.buildCatalogImage(ctx, ref, dockerfileDir, dcDir)
		require.NoError(t, err)
		t.Log(err)
	}

}
*/
