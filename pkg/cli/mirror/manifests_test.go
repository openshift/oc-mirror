package mirror

import (
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
