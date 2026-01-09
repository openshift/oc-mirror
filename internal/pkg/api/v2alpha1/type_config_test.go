package v2alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidPathComponent(t *testing.T) {
	tests := []struct {
		name           string
		targetCatalog  string
		expectedResult bool
	}{
		{
			name:           "Valid path component",
			targetCatalog:  "my-namespace/my-target-name",
			expectedResult: true,
		},
		{
			name:           "Invalid path component - has tag",
			targetCatalog:  "my-namespace/my-target-name:v4.10",
			expectedResult: false,
		},
		{
			name:           "Invalid path component - has digest",
			targetCatalog:  "my-namespace/my-target-name@sha256:v4.10",
			expectedResult: false,
		},
		{
			name:           "Invalid path component with special characters",
			targetCatalog:  "my$namespace/my-target-name",
			expectedResult: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := IsValidPathComponent(test.targetCatalog)
			assert.Equal(t, test.expectedResult, result)
		})
	}
}

func TestGetUniqueNameWithTarget(t *testing.T) {
	type spec struct {
		name       string
		sourceName string
		targetPath string
		targetTag  string
		expected   string
		expError   string
	}

	cases := []spec{
		{
			name:       "Valid/NoTargetPathOrTag",
			sourceName: "registry.io/namespace/image:v1.0",
			targetPath: "",
			targetTag:  "",
			expected:   "registry.io/namespace/image:v1.0",
		},
		{
			name:       "Valid/WithTargetTagOnly",
			sourceName: "registry.io/namespace/image:v1.0",
			targetPath: "",
			targetTag:  "v2.0",
			expected:   "registry.io/namespace/image:v2.0",
		},
		{
			name:       "Valid/WithTargetPathOnly",
			sourceName: "registry.io/namespace/image:v1.0",
			targetPath: "custom-namespace/custom-image",
			targetTag:  "",
			expected:   "registry.io/custom-namespace/custom-image:v1.0",
		},
		{
			name:       "Valid/WithBothTargetPathAndTag",
			sourceName: "registry.io/namespace/image:v1.0",
			targetPath: "custom-namespace/custom-image",
			targetTag:  "v2.0",
			expected:   "registry.io/custom-namespace/custom-image:v2.0",
		},
		{
			name:       "Valid/WithDigest",
			sourceName: "registry.io/namespace/image@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			targetPath: "",
			targetTag:  "",
			expected:   "registry.io/namespace/image@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		},
		{
			name:       "Valid/WithDigestAndTargetPath",
			sourceName: "registry.io/namespace/image@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			targetPath: "new-namespace/new-image",
			targetTag:  "",
			expected:   "registry.io/new-namespace/new-image@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		},
		{
			name:       "Valid/SinglePathComponent",
			sourceName: "registry.io/simple-image:latest",
			targetPath: "new-image",
			targetTag:  "",
			expected:   "registry.io/new-image:latest",
		},
		{
			name:       "Valid/MultiLevelNamespace",
			sourceName: "registry.io/ns1/ns2/image:tag",
			targetPath: "new-ns1/new-ns2/new-ns3/new-image",
			targetTag:  "",
			expected:   "registry.io/new-ns1/new-ns2/new-ns3/new-image:tag",
		},
		{
			name:       "Valid/TargetPathWithDots",
			sourceName: "registry.io/namespace/image:v1.0",
			targetPath: "my.namespace/my.image",
			targetTag:  "",
			expected:   "registry.io/my.namespace/my.image:v1.0",
		},
		{
			name:       "Valid/TargetPathWithUnderscores",
			sourceName: "registry.io/namespace/image:v1.0",
			targetPath: "my_namespace/my_image",
			targetTag:  "",
			expected:   "registry.io/my_namespace/my_image:v1.0",
		},
		{
			name:       "Valid/TargetPathWithDoubleUnderscores",
			sourceName: "registry.io/namespace/image:v1.0",
			targetPath: "my__namespace/my__image",
			targetTag:  "",
			expected:   "registry.io/my__namespace/my__image:v1.0",
		},
		{
			name:       "Valid/TargetPathWithHyphens",
			sourceName: "registry.io/namespace/image:v1.0",
			targetPath: "my-namespace/my-image",
			targetTag:  "",
			expected:   "registry.io/my-namespace/my-image:v1.0",
		},
		{
			name:       "Invalid/TargetPathWithTag",
			sourceName: "registry.io/namespace/image:v1.0",
			targetPath: "custom-namespace/custom-image:v2.0",
			targetTag:  "",
			expError:   "invalid target path component \"custom-namespace/custom-image:v2.0\": should not contain a tag or digest. Expected format is 1 or more path components separated by /, where each path component is a set of alpha-numeric and regexp (?:[._]|__|[-]*). For more, see https://github.com/containers/image/blob/main/docker/reference/regexp.go",
		},
		{
			name:       "Invalid/TargetPathWithDigest",
			sourceName: "registry.io/namespace/image:v1.0",
			targetPath: "custom-namespace/custom-image@sha256:abcd",
			targetTag:  "",
			expError:   "invalid target path component \"custom-namespace/custom-image@sha256:abcd\": should not contain a tag or digest. Expected format is 1 or more path components separated by /, where each path component is a set of alpha-numeric and regexp (?:[._]|__|[-]*). For more, see https://github.com/containers/image/blob/main/docker/reference/regexp.go",
		},
		{
			name:       "Invalid/TargetPathWithInvalidCharacters",
			sourceName: "registry.io/namespace/image:v1.0",
			targetPath: "custom$namespace/custom-image",
			targetTag:  "",
			expError:   "invalid target path component \"custom$namespace/custom-image\": should not contain a tag or digest. Expected format is 1 or more path components separated by /, where each path component is a set of alpha-numeric and regexp (?:[._]|__|[-]*). For more, see https://github.com/containers/image/blob/main/docker/reference/regexp.go",
		},
		{
			name:       "Invalid/MalformedSourceName",
			sourceName: "whatever",
			targetPath: "",
			targetTag:  "",
			expError:   "unable to parse image correctly",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := getUniqueNameWithTarget(c.sourceName, c.targetPath, c.targetTag)
			if c.expError != "" {
				if assert.Error(t, err) {
					assert.Contains(t, err.Error(), c.expError)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, c.expected, result)
			}
		})
	}
}

func TestOperatorGetUniqueName(t *testing.T) {
	type spec struct {
		name     string
		operator Operator
		expected string
		expError string
	}

	cases := []spec{
		{
			name: "Valid/CatalogOnly",
			operator: Operator{
				Catalog: "registry.io/namespace/catalog:v1.0",
			},
			expected: "registry.io/namespace/catalog:v1.0",
		},
		{
			name: "Valid/WithTargetCatalog",
			operator: Operator{
				Catalog:       "registry.io/namespace/catalog:v1.0",
				TargetCatalog: "custom-namespace/custom-catalog",
			},
			expected: "registry.io/custom-namespace/custom-catalog:v1.0",
		},
		{
			name: "Valid/WithTargetTag",
			operator: Operator{
				Catalog:   "registry.io/namespace/catalog:v1.0",
				TargetTag: "v2.0",
			},
			expected: "registry.io/namespace/catalog:v2.0",
		},
		{
			name: "Valid/WithBothTargetCatalogAndTag",
			operator: Operator{
				Catalog:       "registry.io/namespace/catalog:v1.0",
				TargetCatalog: "custom-namespace/custom-catalog",
				TargetTag:     "v2.0",
			},
			expected: "registry.io/custom-namespace/custom-catalog:v2.0",
		},
		{
			name: "Invalid/InvalidTargetCatalog",
			operator: Operator{
				Catalog:       "registry.io/namespace/catalog:v1.0",
				TargetCatalog: "invalid:tag",
			},
			expError: "invalid target path component",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := c.operator.GetUniqueName()
			if c.expError != "" {
				if assert.Error(t, err) {
					assert.Contains(t, err.Error(), c.expError)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, c.expected, result)
			}
		})
	}
}

func TestImageGetUniqueName(t *testing.T) {
	type spec struct {
		name     string
		image    Image
		expected string
		expError string
	}

	cases := []spec{
		{
			name: "Valid/ImageNameOnly",
			image: Image{
				Name: "registry.io/namespace/image:v1.0",
			},
			expected: "registry.io/namespace/image:v1.0",
		},
		{
			name: "Valid/WithTargetRepo",
			image: Image{
				Name:       "registry.io/namespace/image:v1.0",
				TargetRepo: "custom-namespace/custom-image",
			},
			expected: "registry.io/custom-namespace/custom-image:v1.0",
		},
		{
			name: "Valid/WithTargetTag",
			image: Image{
				Name:      "registry.io/namespace/image:v1.0",
				TargetTag: "v2.0",
			},
			expected: "registry.io/namespace/image:v2.0",
		},
		{
			name: "Valid/WithBothTargetRepoAndTag",
			image: Image{
				Name:       "registry.io/namespace/image:v1.0",
				TargetRepo: "custom-namespace/custom-image",
				TargetTag:  "v2.0",
			},
			expected: "registry.io/custom-namespace/custom-image:v2.0",
		},
		{
			name: "Invalid/InvalidTargetRepo",
			image: Image{
				Name:       "registry.io/namespace/image:v1.0",
				TargetRepo: "invalid:tag",
			},
			expError: "invalid target path component",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := c.image.GetUniqueName()
			if c.expError != "" {
				if assert.Error(t, err) {
					assert.Contains(t, err.Error(), c.expError)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, c.expected, result)
			}
		})
	}
}
