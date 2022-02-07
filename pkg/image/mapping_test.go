package image

import (
	"testing"

	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/stretchr/testify/require"
)

func TestByCategory(t *testing.T) {
	tests := []struct {
		name     string
		input    TypedImageMapping
		typ      []ImageType
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
				Category: TypeOperatorBundle}: {
				TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: TypeOperatorBundle},
			{TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
				Category: TypeOCPRelease}: {
				TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: TypeOCPRelease},
		},
		typ: []ImageType{TypeOperatorBundle},
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
			Category: TypeOperatorBundle}: {
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "disconn-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: TypeOperatorBundle},
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
			Category: TypeOperatorBundle}: {
			TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "disconn-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
			Category: TypeOperatorBundle},
		},
		typ:      []ImageType{TypeGeneric},
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
				Category: TypeOperatorBundle}: {
				TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: TypeOperatorBundle},
			{TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
				Category: TypeOperatorCatalog}: {
				TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: TypeOperatorCatalog},
			{TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
				Category: TypeGeneric}: {
				TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: TypeGeneric},
		},
		typ: []ImageType{TypeOperatorBundle, TypeOperatorCatalog},
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
				Category: TypeOperatorBundle}: {
				TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: TypeOperatorBundle},
			{TypedImageReference: imagesource.TypedImageReference{
				Ref: reference.DockerImageReference{
					Registry:  "some-registry",
					Namespace: "namespace",
					Name:      "image",
					ID:        "digest",
				},
				Type: imagesource.DestinationRegistry,
			},
				Category: TypeOperatorCatalog}: {
				TypedImageReference: imagesource.TypedImageReference{
					Ref: reference.DockerImageReference{
						Registry:  "disconn-registry",
						Namespace: "namespace",
						Name:      "image",
						ID:        "digest",
					},
					Type: imagesource.DestinationRegistry,
				},
				Category: TypeOperatorCatalog},
		},
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mapping := ByCategory(test.input, test.typ...)
			require.Equal(t, test.expected, mapping)
		})
	}
}
