package mirror

import (
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart/loader"
	"k8s.io/client-go/util/jsonpath"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func Test_GetCustomPaths(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  int
	}{
		{
			name:  "tests no additional paths",
			paths: []string{},
			want:  4,
		},
		{
			name: "tests custom paths",
			paths: []string{
				"{.spec.template.spec.tests[*].image}",
			},
			want: 5,
		},
	}
	for _, tt := range tests {

		paths := getImagesPath(tt.paths...)

		if len(paths) != tt.want {
			t.Errorf("in %s, expect to get %d, got %d", tt.name, tt.want, len(paths))
		}
	}

}

func Test_Search(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		ipaths  []string
		want    []v1alpha1.AdditionalImages
		wantErr bool
	}{
		{
			name:   "test podinfo",
			path:   "testdata/artifacts/podinfo.yaml",
			ipaths: getImagesPath(),
			want: []v1alpha1.AdditionalImages{
				{Image: v1alpha1.Image{
					Name: "ghcr.io/stefanprodan/podinfo:6.0.0"}},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {

		b, err := ioutil.ReadFile(tt.path)
		require.NoError(t, err)

		imgs, err := search(b, tt.ipaths...)
		require.NoError(t, err)

		if !reflect.DeepEqual(imgs, tt.want) {
			if !tt.wantErr {
				t.Errorf(`in %s, expect to get "%s", got "%s"`, tt.name, tt.want, imgs)
			}
		}
	}
}

func Test_ParseJSON(t *testing.T) {

	types := map[string]interface{}{
		"images": "test",
	}

	tests := []struct {
		name     string
		template string
		input    interface{}
		want     []string
		wantErr  bool
	}{
		{
			name:     "tests paths",
			template: "{.images }",
			input:    types,
			want:     []string{"test"},
			wantErr:  false,
		},
		{
			name:     "tests failure",
			template: "{.images }",
			input:    types,
			want:     []string{"nottest"},
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		j := jsonpath.New(tt.name)
		j.AllowMissingKeys(true)
		out, err := parseJSONPath(tt.input, j, tt.template)
		require.NoError(t, err)

		if !reflect.DeepEqual(out, tt.want) {
			if !tt.wantErr {
				t.Errorf(`in %s, expect to get "%s", got "%s from %v"`, tt.name, tt.want, out, tt.input)
			}
		}
	}
}

func Test_Render(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		ipaths  []string
		wantErr bool
	}{
		{
			name:    "test podinfo",
			path:    "testdata/artifacts/podinfo-6.0.0.tgz",
			wantErr: false,
		},
	}
	for _, tt := range tests {

		chart, err := loader.Load(tt.path)
		require.NoError(t, err)

		result, err := render(chart)
		require.NoError(t, err)

		if _, err = yaml.Marshal(result); err != nil {
			t.Errorf("in %s, render result want not valid %v", tt.name, err)
		}
	}
}

func Test_FindImages(t *testing.T) {

	ipaths := []string{}
	path := "testdata/artifacts/podinfo-6.0.0.tgz"
	want := []v1alpha1.AdditionalImages{
		{Image: v1alpha1.Image{
			Name: "ghcr.io/stefanprodan/podinfo:6.0.0"}},
	}

	imgs, err := findImages(path, ipaths...)
	require.NoError(t, err)

	if !reflect.DeepEqual(imgs, want) {
		t.Errorf(`in %s, expect to get "%s", got "%s"`, "", want, imgs)
	}
}
