package image

import (
	"testing"

	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/stretchr/testify/require"
)

func TestByCategory(t *testing.T) {
	tests := []struct {
		name     string
		input    TypedImageMapping
		typ      []v1alpha2.ImageType
		expected TypedImageMapping
		err      string
	}{{
		name: "Valid/OneType",
		input: TypedImageMapping{
			{TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
				Category: v1alpha2.TypeOperatorBundle}: {
				TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeOperatorBundle},
			{TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
				Category: v1alpha2.TypeOCPRelease}: {
				TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeOCPRelease},
		},
		typ: []v1alpha2.ImageType{v1alpha2.TypeOperatorBundle},
		expected: TypedImageMapping{{
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeOperatorBundle}: {
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "disconn-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeOperatorBundle},
		},
	}, {
		name: "Valid/PruneAllTypes",
		input: TypedImageMapping{{
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeOperatorBundle}: {
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "disconn-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeOperatorBundle},
		},
		typ:      []v1alpha2.ImageType{v1alpha2.TypeGeneric},
		expected: TypedImageMapping{},
	}, {
		name: "Valid/MultipleTypes",
		input: TypedImageMapping{
			{TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
				Category: v1alpha2.TypeOperatorBundle}: {
				TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeOperatorBundle},
			{TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
				Category: v1alpha2.TypeOperatorCatalog}: {
				TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeOperatorCatalog},
			{TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
				Category: v1alpha2.TypeGeneric}: {
				TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeGeneric},
		},
		typ: []v1alpha2.ImageType{v1alpha2.TypeOperatorBundle, v1alpha2.TypeOperatorCatalog},
		expected: TypedImageMapping{
			{TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
				Category: v1alpha2.TypeOperatorBundle}: {
				TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeOperatorBundle},
			{TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
				Category: v1alpha2.TypeOperatorCatalog}: {
				TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: v1alpha2.TypeOperatorCatalog},
		},
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mapping := ByCategory(test.input, test.typ...)
			require.Equal(t, test.expected, mapping)
		})
	}
}

func TestReadImageMapping(t *testing.T) {
	tests := []struct {
		name      string
		seperator string
		path      string
		typ       v1alpha2.ImageType
		expected  TypedImageMapping
		err       string
	}{{
		name:      "Valid/Separator",
		path:      "testdata/mappings/valid.txt",
		seperator: "=",
		typ:       v1alpha2.TypeOperatorBundle,
		expected: TypedImageMapping{{
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry.com",
					Namespace: "namespace",
					Name:      "image",
					Tag:       "latest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeOperatorBundle}: {
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "disconn-registry.com",
					Namespace: "namespace",
					Name:      "image",
					Tag:       "latest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeOperatorBundle},
		},
	}, {
		name:      "Invalid/NoSeparator",
		path:      "testdata/mappings/invalid.txt",
		seperator: "=",
		err:       "mapping \"=\" expected to have exactly one \"some-registry.com/namespace/image:latest==disconn-registry.com/namespace/image:latest\"",
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mapping, err := ReadImageMapping(test.path, test.seperator, test.typ)
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expected, mapping)
			}

		})
	}
}

func TestToRegistry(t *testing.T) {
	toMirror := "test.registry"
	inputMapping := TypedImageMapping{
		{TypedImageReference: imagesource.TypedImageReference{
			Ref: reference.DockerImageReference{
				Registry:  "some-registry",
				Namespace: "namespace",
				Name:      "image",
				ID:        "sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
			},
			Type: imagesource.DestinationRegistry,
		},
			Category: v1alpha2.TypeOperatorBundle}: {
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "disconn-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
				},
				Type: imagesource.DestinationFile,
			},
			Category: v1alpha2.TypeOperatorBundle}}

	expMapping := TypedImageMapping{
		{TypedImageReference: imagesource.TypedImageReference{
			Ref: reference.DockerImageReference{
				Registry:  "some-registry",
				Namespace: "namespace",
				Name:      "image",
				ID:        "sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
			},
			Type: imagesource.DestinationRegistry,
		},
			Category: v1alpha2.TypeOperatorBundle}: {
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "test.registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
					Tag:       "30c794",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: v1alpha2.TypeOperatorBundle}}

	inputMapping.ToRegistry(toMirror, "")
	require.Equal(t, expMapping, inputMapping)
}
