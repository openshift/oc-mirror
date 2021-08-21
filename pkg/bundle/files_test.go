package bundle

import (
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
)

func Test_ReconcilingFiles(t *testing.T) {
	type fields struct {
		files []v1alpha1.File
	}
	tests := []struct {
		name   string
		fields fields
		files  []string
		want   []v1alpha1.File
	}{
		{
			name: "testing basic example",
			fields: fields{
				files: []v1alpha1.File{
					{Name: "test1"},
				},
			},
			files: []string{"test1", "test2"},
			want: []v1alpha1.File{
				{Name: "test2"},
			},
		},
		{
			name: "testing duplicate entries",
			fields: fields{
				files: []v1alpha1.File{
					{Name: "test1"},
				},
			},
			files: []string{"test1", "test2", "test2"},
			want: []v1alpha1.File{
				{Name: "test2"},
			},
		},
	}
	for _, tt := range tests {

		meta := v1alpha1.Metadata{
			MetadataSpec: v1alpha1.MetadataSpec{
				PastFiles: tt.fields.files,
			},
		}

		tmpdir := t.TempDir()

		cwd, err := os.Getwd()

		if err != nil {
			t.Fatal(err)
		}

		if err := os.Chdir(tmpdir); err != nil {
			t.Fatal(err)
		}

		defer os.Chdir(cwd)

		d1 := []byte("hello\ngo\n")

		for _, name := range tt.files {
			if err := ioutil.WriteFile(name, d1, 0644); err != nil {
				t.Fatal(err)
			}
		}

		actual, err := ReconcileFiles(meta, ".")

		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(actual, tt.want) {
			t.Errorf("Test %s: Expected '%v', got '%v'", tt.name, tt.want, actual)
		}

	}
}
