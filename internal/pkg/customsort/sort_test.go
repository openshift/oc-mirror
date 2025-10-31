package customsort

import (
	"sort"
	"testing"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/stretchr/testify/assert"
)

func TestSort(t *testing.T) {

	type testCase struct {
		caseName       string
		allImages      []v2alpha1.CopyImageSchema
		expectedOutput []v2alpha1.CopyImageSchema
	}

	testCases := []testCase{
		{
			caseName: "Sort - should sort the images based on the priority",
			allImages: []v2alpha1.CopyImageSchema{
				{
					Type: v2alpha1.TypeOperatorCatalog,
				},
				{
					Type: v2alpha1.TypeOperatorRelatedImage,
				},
				{
					Type: v2alpha1.TypeOperatorBundle,
				},
				{
					Type: v2alpha1.TypeOCPRelease,
				},
				{
					Type: v2alpha1.TypeKubeVirtContainer,
				},
				{
					Type: v2alpha1.TypeOCPReleaseContent,
				},
				{
					Type: v2alpha1.TypeCincinnatiGraph,
				},
				{
					Type: v2alpha1.TypeGeneric,
				},
				{
					Type: v2alpha1.TypeHelmImage,
				},
			},
			expectedOutput: []v2alpha1.CopyImageSchema{
				{
					Type: v2alpha1.TypeOCPReleaseContent,
				},
				{
					Type: v2alpha1.TypeKubeVirtContainer,
				},
				{
					Type: v2alpha1.TypeOCPRelease,
				},
				{
					Type: v2alpha1.TypeCincinnatiGraph,
				},
				{
					Type: v2alpha1.TypeOperatorRelatedImage,
				},
				{
					Type: v2alpha1.TypeGeneric,
				},
				{
					Type: v2alpha1.TypeHelmImage,
				},
				{
					Type: v2alpha1.TypeOperatorBundle,
				},
				{
					Type: v2alpha1.TypeOperatorCatalog,
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			sort.Sort(ByTypePriority(testCase.allImages))
			assert.Equal(t, testCase.expectedOutput, testCase.allImages)
		})
	}
}
