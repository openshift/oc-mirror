package mirror

import (
	"os"
	"path/filepath"
	"testing"

	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/image"
)

func TestICSPGeneration(t *testing.T) {
	tests := []struct {
		name          string
		sourceImage   image.TypedImage
		destImage     image.TypedImage
		typ           ICSPBuilder
		icspScope     string
		icspSizeLimit int
		expected      []operatorv1alpha1.ImageContentSourcePolicy
		err           string
	}{{
		name: "Valid/OperatorType",
		sourceImage: image.TypedImage{
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeOperatorBundle,
		},
		destImage: image.TypedImage{
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "disconn-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeOperatorBundle,
		},
		icspScope:     "repository",
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
						Source:  "some-registry/namespace/image",
						Mirrors: []string{"disconn-registry/namespace/image"},
					},
				},
			},
		},
		},
	}, {
		name: "Valid/GenericType",
		sourceImage: image.TypedImage{
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeGeneric,
		},
		destImage: image.TypedImage{
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "disconn-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeGeneric,
		},
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
		sourceImage: image.TypedImage{
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeOCPRelease,
		},
		destImage: image.TypedImage{
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "disconn-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeOCPRelease,
		},
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
		},
		},
	}, {
		name: "Valid/NamespaceScope",
		sourceImage: image.TypedImage{
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeGeneric,
		},
		destImage: image.TypedImage{
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "disconn-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeGeneric,
		},
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
		sourceImage: image.TypedImage{
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeGeneric,
		},
		destImage: image.TypedImage{
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "disconn-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeGeneric,
		},
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
		sourceImage: image.TypedImage{
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeGeneric,
		},
		destImage: image.TypedImage{
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "disconn-registry",
					Namespace: "",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeGeneric,
		},
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
		sourceImage: image.TypedImage{
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeGeneric,
		},
		icspScope:     "namespace",
		icspSizeLimit: 250000,
		typ:           &GenericBuilder{},
		destImage: image.TypedImage{
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "disconn-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeGeneric,
		},
		expected: nil,
	}, {
		name: "Invalid/InvalidICSPScope",
		sourceImage: image.TypedImage{
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeGeneric,
		},
		icspScope:     "invalid",
		icspSizeLimit: 250000,
		typ:           &GenericBuilder{},
		destImage: image.TypedImage{
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "disconn-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeGeneric,
		},
		expected: nil,
		err:      "invalid ICSP scope invalid",
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mapping := image.TypedImageMapping{}
			mapping[test.sourceImage] = test.destImage
			icsps, err := GenerateICSP("test", test.icspScope, test.icspSizeLimit, mapping, test.typ)
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expected, icsps)
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
				{TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry: "test-registry",
						Name:     "dev",
						Tag:      "latest",
					},
					Type: imagesource.DestinationRegistry,
				},
					Category: v1alpha2.TypeOperatorCatalog}: {
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Registry: "test-registry",
							Name:     "dev",
							Tag:      "latest",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeOperatorCatalog},
				{TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry: "test-registry",
						Name:     "staging",
						Tag:      "v1",
					},
					Type: imagesource.DestinationRegistry,
				},
					Category: v1alpha2.TypeOperatorCatalog}: {
					TypedImageReference: imagesource.TypedImageReference{
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
				{TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry: "test-registry",
						Name:     "dev",
						Tag:      "latest",
					},
					Type: imagesource.DestinationRegistry,
				},
					Category: v1alpha2.TypeOperatorCatalog}: {
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Registry: "test-registry",
							Name:     "dev",
							Tag:      "latest",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeOperatorCatalog},
				{TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry: "test-registry",
						Name:     "dev",
						Tag:      "v1",
					},
					Type: imagesource.DestinationRegistry,
				},
					Category: v1alpha2.TypeOperatorCatalog}: {
					TypedImageReference: imagesource.TypedImageReference{
						Ref: reference.DockerImageReference{
							Registry: "test-registry",
							Name:     "dev",
							Tag:      "v1",
						},
						Type: imagesource.DestinationRegistry,
					},
					Category: v1alpha2.TypeOperatorCatalog},
				{TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry: "test-registry",
						Name:     "dev",
						Tag:      "v2",
					},
					Type: imagesource.DestinationRegistry,
				},
					Category: v1alpha2.TypeOperatorCatalog}: {
					TypedImageReference: imagesource.TypedImageReference{
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
				{TypedImageReference: imagesource.TypedImageReference{
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
					TypedImageReference: imagesource.TypedImageReference{
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
