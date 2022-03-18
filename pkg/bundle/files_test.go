package bundle

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/stretchr/testify/require"
)

func TestReconcileV2Dir(t *testing.T) {
	type fields struct {
		files     []string
		dirPaths  []string
		filePaths []string
		path      string
	}
	tests := []struct {
		name          string
		fields        fields
		wantBlobs     []string
		wantManifests []string
		wantErr       func(string) string
	}{
		{
			name: "Valid/FirstRun",
			fields: fields{
				files: []string{},
				dirPaths: []string{
					filepath.Join("v2", "test", "blobs"),
					filepath.Join("v2", "test", "manifests"),
					"internal",
				},
				filePaths: []string{
					filepath.Join("v2", "test", "blobs", "test1"),
					filepath.Join("internal", "test2"),
					filepath.Join("v2", "test", "blobs", "test3"),
					filepath.Join("v2", "test", "manifests", "test4"),
				},
				path: "v2",
			},
			wantBlobs:     []string{"test1", "test3"},
			wantManifests: []string{filepath.Join("v2", "test", "manifests", "test4")},
		},
		{
			name: "Valid/DifferentialRun",
			fields: fields{
				files: []string{"test1"},
				dirPaths: []string{
					filepath.Join("v2", "test", "blobs"),
					filepath.Join("v2", "test", "manifests"),
					"internal",
				},
				filePaths: []string{
					filepath.Join("v2", "test", "blobs", "test1"),
					filepath.Join("internal", "test2"),
					filepath.Join("v2", "test", "blobs", "test3"),
					filepath.Join("v2", "test", "manifests", "test4"),
				},
				path: "v2",
			},
			wantBlobs:     []string{"test3"},
			wantManifests: []string{filepath.Join("v2", "test", "manifests", "test4")},
		},
		{
			name: "Invalid/PathNameNotV2",
			fields: fields{
				files: []string{},
				dirPaths: []string{
					filepath.Join("v2", "test", "blobs"),
					filepath.Join("v2", "test", "manifests"),
					"internal",
				},
				filePaths: []string{
					filepath.Join("v2", "test", "blobs", "test1"),
					filepath.Join("internal", "test2"),
					filepath.Join("v2", "test", "blobs", "test3"),
					filepath.Join("v2", "test", "manifests", "test4"),
				},
				path: "",
			},
			wantBlobs:     []string{},
			wantManifests: []string{},
			wantErr: func(s string) string {
				return fmt.Sprintf("path %q is not a v2 directory", s)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assocs := image.AssociationSet{"imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": image.Associations{
				"imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": {
					Name:            "imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Path:            "single_manifest",
					TagSymlink:      "latest",
					ID:              "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Type:            v1alpha2.TypeGeneric,
					ManifestDigests: nil,
					LayerDigests:    test.fields.files,
				},
			},
			}

			tmpdir := t.TempDir()
			require.NoError(t, prepFiles(tmpdir, test.fields.dirPaths, test.fields.filePaths))
			filenames := map[string]string{filepath.Join(tmpdir, test.fields.path): "v2"}
			actualManifests, actualBlobs, err := ReconcileV2Dir(assocs, filenames)
			if test.wantErr != nil {
				require.EqualError(t, err, test.wantErr(tmpdir))
			} else {
				require.NoError(t, err)
				require.Equal(t, test.wantBlobs, actualBlobs)
				require.Equal(t, test.wantManifests, actualManifests)
			}
		})
	}
}

func prepFiles(root string, paths []string, files []string) error {
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Join(root, path), os.ModePerm); err != nil {
			return err
		}
	}
	d1 := []byte("hello\ngo\n")
	for _, file := range files {
		if err := ioutil.WriteFile(filepath.Join(root, file), d1, 0644); err != nil {
			return err
		}
	}
	return nil
}
