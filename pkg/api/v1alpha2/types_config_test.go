package v1alpha2

import (
	"sigs.k8s.io/yaml"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReleaseChannel_UnmarshalJSON(t *testing.T) {
	type args struct {
		data []byte
	}
	tests := []struct {
		name        string
		expected    ReleaseChannel
		args        args
		expectedErr string
	}{
		{
			name:     "Valid/EmptyFields",
			expected: ReleaseChannel{},
			args:     args{},
		},
		{
			name: "Valid/AllFieldsPresent",
			expected: ReleaseChannel{
				Name:         "name",
				Type:         TypeOKD,
				MinVersion:   "1.2.3",
				MaxVersion:   "4.5.6",
				ShortestPath: true,
				Full:         true,
			},
			args: args{data: []byte(`
name: name
type: okd
minVersion: 1.2.3
maxVersion: 4.5.6
shortestPath: true
full: true
`)},
		},
		{
			name: "Valid/SemverMajorMinor",
			expected: ReleaseChannel{
				Name:       "name",
				MinVersion: "1.2.0",
				MaxVersion: "4.5.0",
			},
			args: args{data: []byte(`
name: name
minVersion: 1.2
maxVersion: 4.5
`)},
		},
		{
			name: "Valid/SemverMajor",
			expected: ReleaseChannel{
				Name:       "name",
				MaxVersion: "1.0.0",
			},
			args: args{data: []byte(`
name: name
maxVersion: 1
`)},
		},
		{
			name:     "Invalid/NonSemver",
			expected: ReleaseChannel{},
			args: args{data: []byte(`
name: name
minVersion: 1.2.3.4
`)},
			expectedErr: "error unmarshaling JSON: while decoding JSON: unable to parse config; minVersion must be a version if present: Invalid character(s) found in patch number \"3.4\"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var actual = ReleaseChannel{}
			err := yaml.Unmarshal(tt.args.data, &actual)
			if err != nil && tt.expectedErr != "" {
				require.EqualError(t, err, tt.expectedErr)
			}
			require.Equal(t, tt.expected, actual)
		})
	}
}
