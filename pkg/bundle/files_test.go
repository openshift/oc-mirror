package bundle

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/stretchr/testify/require"
)

func TestReconcileV2Dir(t *testing.T) {
	type fields struct {
		files     []v1alpha1.Blob
		dirPaths  []string
		filePaths []string
		path      string
	}
	tests := []struct {
		name          string
		fields        fields
		wantBlobs     []v1alpha1.Blob
		wantManifests []v1alpha1.Manifest
		wantErr       func(string) string
	}{
		{
			name: "Valid/FirstRun",
			fields: fields{
				files: []v1alpha1.Blob{},
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
			wantBlobs: []v1alpha1.Blob{
				{ID: "test1", NamespaceName: "test"},
				{ID: "test3", NamespaceName: "test"},
			},
			wantManifests: []v1alpha1.Manifest{
				{
					Name:          filepath.Join("v2", "test", "manifests", "test4"),
					NamespaceName: "test",
				},
			},
		},
		{
			name: "Valid/DifferentialRun",
			fields: fields{
				files: []v1alpha1.Blob{
					{ID: "test1", NamespaceName: "test"},
				},
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
			wantBlobs: []v1alpha1.Blob{
				{ID: "test3", NamespaceName: "test"},
			},
			wantManifests: []v1alpha1.Manifest{
				{
					Name:          filepath.Join("v2", "test", "manifests", "test4"),
					NamespaceName: "test",
				},
			},
		},
		{
			name: "Invalid/PathNameNotV2",
			fields: fields{
				files: []v1alpha1.Blob{},
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
			wantBlobs:     []v1alpha1.Blob{},
			wantManifests: []v1alpha1.Manifest{},
			wantErr: func(s string) string {
				return fmt.Sprintf("path %q is not a v2 directory", s)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			meta := v1alpha1.Metadata{
				MetadataSpec: v1alpha1.MetadataSpec{
					PastBlobs: test.fields.files,
				},
			}
			tmpdir := t.TempDir()
			require.NoError(t, prepFiles(tmpdir, test.fields.dirPaths, test.fields.filePaths))
			filenames := map[string]string{filepath.Join(tmpdir, test.fields.path): "v2"}
			actualManifests, actualBlobs, err := ReconcileV2Dir(meta, filenames)
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
