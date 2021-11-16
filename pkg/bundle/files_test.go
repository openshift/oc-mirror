package bundle

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/openshift/oc/pkg/cli/image/mirror"
	"github.com/sirupsen/logrus"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func Test_ReconcilingBlobs(t *testing.T) {

	paths := []string{
		filepath.Join("v2", "blobs"),
		filepath.Join("v2", "manifests"),
		"blobs",
		"internal",
	}

	type fields struct {
		files []v1alpha1.Blob
	}
	tests := []struct {
		name   string
		fields fields
		want   []v1alpha1.Blob
	}{
		{
			name: "testing pulling new blobs",
			fields: fields{
				files: []v1alpha1.Blob{
					{ID: "test1", NamespaceName: "foo/bar"},
				},
			},
			want: []v1alpha1.Blob{
				{ID: "test3", NamespaceName: "test3"},
			},
		},
	}
	for _, tt := range tests {
		meta := v1alpha1.Metadata{
			MetadataSpec: v1alpha1.MetadataSpec{
				PastBlobs: tt.fields.files,
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

		for _, path := range paths {
			if err := os.MkdirAll(path, os.ModePerm); err != nil {
				t.Fatal(err)
			}
		}

		// Write files
		d1 := []byte("hello\ngo\n")
		if err := ioutil.WriteFile("v2/test1", d1, 0644); err != nil {
			t.Fatal(err)
		}
		if err := ioutil.WriteFile("internal/test2", d1, 0644); err != nil {
			t.Fatal(err)
		}
		if err := ioutil.WriteFile("v2/test3", d1, 0644); err != nil {
			t.Fatal(err)
		}
		if err := ioutil.WriteFile("v2/manifests/test4", d1, 0644); err != nil {
			t.Fatal(err)
		}

		actual, err := ReconcileBlobs(meta)

		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(actual, tt.want) {
			t.Errorf("Test %s: Expected '%v', got '%v'", tt.name, tt.want, actual)
		}

	}
}

func Test_ReconcilingManifest(t *testing.T) {

	paths := []string{
		filepath.Join("v2", "blobs"),
		filepath.Join("v2", "manifests"),
		"manifests",
	}

	type fields struct {
		files []v1alpha1.Manifest
	}
	tests := []struct {
		name   string
		fields fields
		want   []v1alpha1.Manifest
	}{
		{
			name: "testing new manifests",
			fields: fields{
				files: []v1alpha1.Manifest{
					{Name: "v2"},
					{Name: "v2/manifests"},
					{Name: "v2/manifests/test1"},
					{Name: "v2/manifests/test2"},
				},
			},
			want: []v1alpha1.Manifest{
				{Name: "v2"},
				{Name: "v2/manifests"},
				{Name: "v2/manifests/test1"},
				{Name: "v2/manifests/test2"},
			},
		},
	}
	for _, tt := range tests {

		tmpdir := t.TempDir()

		cwd, err := os.Getwd()

		if err != nil {
			t.Fatal(err)
		}

		if err := os.Chdir(tmpdir); err != nil {
			t.Fatal(err)
		}

		defer os.Chdir(cwd)

		// Write out blobs directroy to ensure these files are skipped
		for _, path := range paths {
			if err := os.MkdirAll(path, os.ModePerm); err != nil {
				t.Fatal(err)
			}
		}

		// Wtie files
		d1 := []byte("hello\ngo\n")
		if err := ioutil.WriteFile("v2/manifests/test1", d1, 0644); err != nil {
			t.Fatal(err)
		}
		if err := ioutil.WriteFile("v2/manifests/test2", d1, 0644); err != nil {
			t.Fatal(err)
		}
		if err := ioutil.WriteFile("v2/blobs/test3", d1, 0644); err != nil {
			t.Fatal(err)
		}

		actual, err := ReconcileManifests()

		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(actual, tt.want) {
			t.Errorf("Test %s: Expected '%v', got '%v'", tt.name, tt.want, actual)
		}

	}
}

func Test_HasCorrupt(t *testing.T) {
	// Create tempdir
	dir := t.TempDir()

	opts := mirror.NewMirrorImageOptions(genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	})
	opts.SecurityOptions.Insecure = true
	opts.SecurityOptions.SkipVerification = true
	opts.FileDir = filepath.Join(dir, config.SourceDir)
	var mappings []mirror.Mapping

	// Get source image information
	srcRef, err := imagesource.ParseReference("docker.io/library/alpine:latest")
	if err != nil {
		logrus.Errorf("error parsing source image %s: %v", srcRef.Ref.Name, err)
	}

	// Set destination image information
	dstRef := srcRef
	dstRef.Type = imagesource.DestinationFile
	dstRef.Ref = dstRef.Ref.DockerClientDefaults()

	// Create mapping from source and destination images
	mappings = append(mappings, mirror.Mapping{
		Source:      srcRef,
		Destination: dstRef,
		Name:        srcRef.Ref.Name,
	})
	opts.Mappings = mappings

	err = opts.Run()
	if err != nil {
		logrus.Error(err)
	}
	/*
		err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				logrus.Error(err)
				return err
			}
			if info.IsDir() && info.Name() == "manifests" {
				return filepath.SkipDir
			}
			file := filepath.Join(path + info.Name())
			if !info.IsDir() {
				os.Rename(file, file+".download")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	*/
	err = HasCorrupt(dir)
	t.Log(err)
	/*
		if !errors.Is(err, ErrCorruptFile) {
			t.Fatal(err)
		} else
			t.Log(err)
		}*/

}
