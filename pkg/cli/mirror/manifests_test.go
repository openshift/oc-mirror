package mirror

import (
	"os"
	"path/filepath"
	"testing"

	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/image"
)

func TestICSPGeneration(t *testing.T) {
	tests := []struct {
		name          string
		sourceImages  []image.TypedImage
		destImages    []image.TypedImage
		typ           ICSPBuilder
		icspScope     string
		icspSizeLimit int
		expected      []operatorv1alpha1.ImageContentSourcePolicy
		err           string
	}{{
		name: "Valid/OperatorType",
		sourceImages: []image.TypedImage{{
			TypedImageReference: image.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeOperatorBundle,
		}},
		destImages: []image.TypedImage{{
			TypedImageReference: image.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "disconn-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeOperatorBundle,
		}},
		icspScope:     "namespace",
		icspSizeLimit: 250000,
		typ:           &OperatorBuilder{},
		expected: []operatorv1alpha1.ImageContentSourcePolicy{{
			TypeMeta: metav1.TypeMeta{
				APIVersion: operatorv1alpha1.GroupVersion.String(),
				Kind:       "ImageContentSourcePolicy"},
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-0",
				Labels: map[string]string{"operators.openshift.org/catalog": "true"},
			},
			Spec: operatorv1alpha1.ImageContentSourcePolicySpec{
				RepositoryDigestMirrors: []operatorv1alpha1.RepositoryDigestMirrors{
					{
						Source:  "some-registry/namespace",
						Mirrors: []string{"disconn-registry/namespace"},
					},
				},
			},
		},
		},
	},
		{
			name: "Valid/GenericType",
			sourceImages: []image.TypedImage{{
				TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "some-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeGeneric,
			}},
			destImages: []image.TypedImage{{
				TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeGeneric,
			}},
			icspScope:     "repository",
			icspSizeLimit: 250000,
			typ:           &GenericBuilder{},
			expected: []operatorv1alpha1.ImageContentSourcePolicy{{
				TypeMeta: metav1.TypeMeta{
					APIVersion: operatorv1alpha1.GroupVersion.String(),
					Kind:       "ImageContentSourcePolicy"},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-0",
				},
				Spec: operatorv1alpha1.ImageContentSourcePolicySpec{
					RepositoryDigestMirrors: []operatorv1alpha1.RepositoryDigestMirrors{
						{
							Source:  "some-registry/namespace/image",
							Mirrors: []string{"disconn-registry/namespace/image"},
						},
					},
				},
			},
			},
		}, {
			name: "Valid/ReleaseType",
			sourceImages: []image.TypedImage{{
				TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "some-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeOCPRelease,
			}},
			destImages: []image.TypedImage{{
				TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeOCPRelease,
			}},
			typ:           &ReleaseBuilder{},
			icspScope:     "repository",
			icspSizeLimit: 250000,
			expected: []operatorv1alpha1.ImageContentSourcePolicy{{
				TypeMeta: metav1.TypeMeta{
					APIVersion: operatorv1alpha1.GroupVersion.String(),
					Kind:       "ImageContentSourcePolicy"},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-0",
				},
				Spec: operatorv1alpha1.ImageContentSourcePolicySpec{
					RepositoryDigestMirrors: []operatorv1alpha1.RepositoryDigestMirrors{
						{
							Source:  "some-registry/namespace/image",
							Mirrors: []string{"disconn-registry/namespace/image"},
						},
					},
				},
			}},
		}, {
			name: "Valid/NamespaceScope",
			sourceImages: []image.TypedImage{{
				TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "some-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeGeneric,
			}},
			destImages: []image.TypedImage{{
				TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeGeneric,
			}},
			typ:           &GenericBuilder{},
			icspScope:     "namespace",
			icspSizeLimit: 250000,
			expected: []operatorv1alpha1.ImageContentSourcePolicy{{
				TypeMeta: metav1.TypeMeta{
					APIVersion: operatorv1alpha1.GroupVersion.String(),
					Kind:       "ImageContentSourcePolicy"},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-0",
				},
				Spec: operatorv1alpha1.ImageContentSourcePolicySpec{
					RepositoryDigestMirrors: []operatorv1alpha1.RepositoryDigestMirrors{
						{
							Source:  "some-registry/namespace",
							Mirrors: []string{"disconn-registry/namespace"},
						},
					},
				},
			},
			},
		}, {
			name: "Valid/RegistryScope",
			sourceImages: []image.TypedImage{{
				TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "some-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeGeneric,
			}},
			destImages: []image.TypedImage{{
				TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeGeneric,
			}},
			typ:           &GenericBuilder{},
			icspScope:     "registry",
			icspSizeLimit: 250000,
			expected: []operatorv1alpha1.ImageContentSourcePolicy{{
				TypeMeta: metav1.TypeMeta{
					APIVersion: operatorv1alpha1.GroupVersion.String(),
					Kind:       "ImageContentSourcePolicy"},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-0",
				},
				Spec: operatorv1alpha1.ImageContentSourcePolicySpec{
					RepositoryDigestMirrors: []operatorv1alpha1.RepositoryDigestMirrors{
						{
							Source:  "some-registry",
							Mirrors: []string{"disconn-registry"},
						},
					},
				},
			},
			},
		}, {
			name: "Valid/NamespaceScopeNoNamespace",
			sourceImages: []image.TypedImage{{
				TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "some-registry",
						Namespace: "",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeGeneric,
			}},
			destImages: []image.TypedImage{{
				TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeGeneric,
			}},
			typ:           &GenericBuilder{},
			icspScope:     "namespace",
			icspSizeLimit: 250000,
			expected: []operatorv1alpha1.ImageContentSourcePolicy{{
				TypeMeta: metav1.TypeMeta{
					APIVersion: operatorv1alpha1.GroupVersion.String(),
					Kind:       "ImageContentSourcePolicy"},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-0",
				},
				Spec: operatorv1alpha1.ImageContentSourcePolicySpec{
					RepositoryDigestMirrors: []operatorv1alpha1.RepositoryDigestMirrors{
						{
							Source:  "some-registry/image",
							Mirrors: []string{"disconn-registry/image"},
						},
					},
				},
			},
			},
		}, {
			name: "Invalid/NoDigestMapping",
			sourceImages: []image.TypedImage{{
				TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "some-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeGeneric,
			}},
			icspScope:     "namespace",
			icspSizeLimit: 250000,
			typ:           &GenericBuilder{},
			destImages: []image.TypedImage{{
				TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeGeneric,
			}},
			expected: nil,
		}, {
			name: "Invalid/InvalidICSPScope",
			sourceImages: []image.TypedImage{{
				TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "some-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeGeneric,
			}},
			icspScope:     "invalid",
			icspSizeLimit: 250000,
			typ:           &GenericBuilder{},
			destImages: []image.TypedImage{{
				TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeGeneric,
			}},
			expected: nil,
			err:      "invalid ICSP scope invalid",
		},
		{
			name: "Valid/OperatorTypeWithRelatedImgs",
			sourceImages: []image.TypedImage{
				{
					TypedImageReference: image.TypedImageReference{
						Ref: reference.DockerImageReference{
							Registry:  "some-registry-bundle",
							Namespace: "namespace-bundle",
							Name:      "image-bundle",
							ID:        "digest-bundle",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeOperatorBundle,
				},
				{
					TypedImageReference: image.TypedImageReference{
						Ref: reference.DockerImageReference{
							Registry:  "some-registry-for-related",
							Namespace: "namespace-related",
							Name:      "image-related",
							ID:        "digest-related",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeOperatorRelatedImage,
				}},
			destImages: []image.TypedImage{
				{
					TypedImageReference: image.TypedImageReference{
						Ref: reference.DockerImageReference{
							Registry:  "disconn-registry",
							Namespace: "namespace-bundle",
							Name:      "image-bundle",
							ID:        "digest-bundle",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeOperatorBundle,
				},
				{
					TypedImageReference: image.TypedImageReference{
						Ref: reference.DockerImageReference{
							Registry:  "disconn-registry",
							Namespace: "namespace-related",
							Name:      "image-related",
							ID:        "digest-related",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeOperatorRelatedImage,
				}},
			icspScope:     "namespace",
			icspSizeLimit: 250000,
			typ:           &OperatorBuilder{},
			expected: []operatorv1alpha1.ImageContentSourcePolicy{{
				TypeMeta: metav1.TypeMeta{
					APIVersion: operatorv1alpha1.GroupVersion.String(),
					Kind:       "ImageContentSourcePolicy"},
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-0",
					Labels: map[string]string{"operators.openshift.org/catalog": "true"},
				},
				Spec: operatorv1alpha1.ImageContentSourcePolicySpec{
					RepositoryDigestMirrors: []operatorv1alpha1.RepositoryDigestMirrors{
						{
							Source:  "some-registry-bundle/namespace-bundle",
							Mirrors: []string{"disconn-registry/namespace-bundle"},
						},
						{
							Source:  "some-registry-for-related/namespace-related",
							Mirrors: []string{"disconn-registry/namespace-related"},
						},
					},
				},
			},
			},
		}}

	// test processNestedPaths
	o := &MirrorOptions{
		MaxNestedPaths: 0,
		ToMirror:       "localhost:5000",
		UserNamespace:  "ocpbugs-11922/mirror-release",
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mapping := image.TypedImageMapping{}
			for ind, sourceImage := range test.sourceImages {
				mapping[sourceImage] = test.destImages[ind]
			}

			icsps, err := o.GenerateICSP("test", test.icspScope, test.icspSizeLimit, mapping, test.typ)
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.NoError(t, err)
				// for loop replaces require.Equal(test.expected, icsps): order elements in Spec.RepositoryDigestMirrors
				// was making the test fail

				for ind, icsp := range test.expected {
					require.ElementsMatch(t, icsp.Spec.RepositoryDigestMirrors, icsps[ind].Spec.RepositoryDigestMirrors)
					require.Equal(t, icsp.Labels, icsps[ind].Labels)
					require.Equal(t, icsp.Name, icsps[ind].Name)
				}
			}
		})
	}
}

func TestWriteCatalogSource(t *testing.T) {
	tests := []struct {
		name          string
		images        image.TypedImageMapping
		expectedFiles []string
		err           string
	}{
		{
			name: "Success/UniqueCatalogNames",
			images: image.TypedImageMapping{
				{TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry: "test-registry",
						Name:     "dev",
						Tag:      "latest",
					},
					Type: imagesource.DestinationRegistry,
				},
					Category: v1alpha2.TypeOperatorCatalog}: {
					TypedImageReference: image.TypedImageReference{
						Ref: reference.DockerImageReference{
							Registry: "test-registry",
							Name:     "dev",
							Tag:      "latest",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeOperatorCatalog},
				{TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry: "test-registry",
						Name:     "staging",
						Tag:      "v1",
					},
					Type: imagesource.DestinationRegistry,
				},
					Category: v1alpha2.TypeOperatorCatalog}: {
					TypedImageReference: image.TypedImageReference{
						Ref: reference.DockerImageReference{
							Registry: "test-registry",
							Name:     "staging",
							Tag:      "v1",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeOperatorCatalog},
			},
			expectedFiles: []string{
				"catalogSource-dev.yaml",
				"catalogSource-staging.yaml",
			},
		},
		{
			name: "Success/DuplicateCatalogName",
			images: image.TypedImageMapping{
				{TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry: "test-registry",
						Name:     "dev",
						Tag:      "latest",
					},
					Type: imagesource.DestinationRegistry,
				},
					Category: v1alpha2.TypeOperatorCatalog}: {
					TypedImageReference: image.TypedImageReference{
						Ref: reference.DockerImageReference{
							Registry: "test-registry",
							Name:     "dev",
							Tag:      "latest",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeOperatorCatalog},
				{TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry: "test-registry",
						Name:     "dev",
						Tag:      "v1",
					},
					Type: imagesource.DestinationRegistry,
				},
					Category: v1alpha2.TypeOperatorCatalog}: {
					TypedImageReference: image.TypedImageReference{
						Ref: reference.DockerImageReference{
							Registry: "test-registry",
							Name:     "dev",
							Tag:      "v1",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeOperatorCatalog},
				{TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry: "test-registry",
						Name:     "dev",
						Tag:      "v2",
					},
					Type: imagesource.DestinationRegistry,
				},
					Category: v1alpha2.TypeOperatorCatalog}: {
					TypedImageReference: image.TypedImageReference{
						Ref: reference.DockerImageReference{
							Registry: "test-registry",
							Name:     "dev",
							Tag:      "v2",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeOperatorCatalog},
			},
			expectedFiles: []string{
				"catalogSource-dev.yaml",
				"catalogSource-dev-1.yaml",
				"catalogSource-dev-2.yaml",
			},
		},
		{
			name:   "Success/EmptyMapping",
			images: nil,
		},
		{
			name: "Success/CatalogNameContainingPathComponents",
			images: image.TypedImageMapping{
				{TypedImageReference: image.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "foo.com",
						Namespace: "cp",
						Name:      "test/common-services",
						Tag:       "",
						ID:        "sha256:ef64abd2c4c9acdc433ed4454b008d90891fe18fe33d3a53e7d6104a4a8bf5c5",
					},
					Type: imagesource.DestinationRegistry,
				},
					Category: v1alpha2.TypeOperatorCatalog}: {
					TypedImageReference: image.TypedImageReference{
						Ref: reference.DockerImageReference{
							Registry:  "localhost:5000",
							Namespace: "cp",
							Name:      "test/common-services",
							Tag:       "",
							ID:        "sha256:ef64abd2c4c9acdc433ed4454b008d90891fe18fe33d3a53e7d6104a4a8bf5c5",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeOperatorCatalog},
			},
			expectedFiles: []string{
				"catalogSource-test-common-services.yaml",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tmpdir := t.TempDir()
			err := WriteCatalogSource(test.images, tmpdir)
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.NoError(t, err)
				if test.expectedFiles != nil {
					for _, file := range test.expectedFiles {
						catalogSourceFile := filepath.Join(tmpdir, file)
						_, err := os.Stat(catalogSourceFile)
						require.NoError(t, err)
					}

				}
			}
		})
	}
}

func TestGenerateCatalogSource(t *testing.T) {

	expCfg := `apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: test
  namespace: openshift-marketplace
spec:
  image: registry.com/catalog:latest
  sourceType: grpc
`

	ref, err := reference.Parse("registry.com/catalog:latest")
	require.NoError(t, err)
	data, err := generateCatalogSource("test", ref)
	require.NoError(t, err)
	require.Equal(t, string(data), expCfg)
}

func TestGenerateUpdateService(t *testing.T) {

	expCfg := `apiVersion: updateservice.operator.openshift.io/v1
kind: UpdateService
metadata:
  name: test
spec:
  graphDataImage: registry.com/graph:latest
  releases: registry.com/releases
  replicas: 2
`

	release, err := reference.Parse("registry.com/releases:latest")
	require.NoError(t, err)
	graph, err := reference.Parse("registry.com/graph:latest")
	require.NoError(t, err)
	data, err := generateUpdateService("test", release, graph)
	require.NoError(t, err)
	require.Equal(t, expCfg, string(data))
}

func TestCreateRFC1035NameForCatalogSource(t *testing.T) {
	tests := []struct {
		testName  string
		input     string
		expected  string
		assertion assert.ErrorAssertionFunc
	}{
		{
			testName:  "single path component",
			input:     "abc",
			expected:  "cs-abc",
			assertion: assert.NoError,
		},
		{
			testName:  "two path component",
			input:     "abc/def",
			expected:  "cs-abc-def",
			assertion: assert.NoError,
		},
		{
			testName:  "single path component with uppercase",
			input:     "abC",
			expected:  "cs-abc",
			assertion: assert.NoError,
		},
		{
			testName:  "two path component with uppercase",
			input:     "abc/Def",
			expected:  "cs-abc-def",
			assertion: assert.NoError,
		},
		{
			testName:  "single path component with number",
			input:     "ab0",
			expected:  "cs-ab0",
			assertion: assert.NoError,
		},
		{
			testName:  "two path component with numbers",
			input:     "ab0/de9",
			expected:  "cs-ab0-de9",
			assertion: assert.NoError,
		},
		{
			testName:  "all numbers",
			input:     "123456789",
			expected:  "cs-123456789",
			assertion: assert.NoError,
		},
		{
			testName:  "single path component with non-latin letters",
			input:     "诶比西",
			expected:  "cs-0", // ends in -0 suffix because all chars converted to dashes and de-duped so string would have ended with dash
			assertion: assert.NoError,
		},
		{
			testName:  "two path component with non-latin letters",
			input:     "诶比西/def",
			expected:  "cs-def",
			assertion: assert.NoError,
		},
		{
			testName:  "non-latin letters interspersed through input",
			input:     "诶abc比de西f",
			expected:  "cs-abc-de-f",
			assertion: assert.NoError,
		},
		{
			testName:  "dash only",
			input:     "-",
			expected:  "cs-0", // ends in -0 suffix because string would end with dash, and this is not allowed
			assertion: assert.NoError,
		},
		{
			testName:  "empty string",
			input:     "",
			expected:  "cs-0", // ends in -0 suffix because string would end with dash, and this is not allowed
			assertion: assert.NoError,
		},
		{
			testName:  "bunch of dashes",
			input:     "/////",
			expected:  "cs-0", // ends in -0 suffix because string would end with dash, and this is not allowed
			assertion: assert.NoError,
		},
	}
	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			got, gotErr := createRFC1035NameForCatalogSource(test.input)
			assert.Equal(t, test.expected, got)
			test.assertion(t, gotErr)
		})
	}

}
