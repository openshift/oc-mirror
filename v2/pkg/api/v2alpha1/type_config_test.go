package v2alpha1

import "testing"

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
			if result != test.expectedResult {
				t.Errorf("Expected %v, but got %v", test.expectedResult, result)
			}
		})
	}
}
