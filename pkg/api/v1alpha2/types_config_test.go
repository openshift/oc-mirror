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
			name:     "Invalid/MajorMinorVersionOnly",
			expected: ReleaseChannel{},
			args: args{data: []byte(`
minVersion: 1.2
maxVersion: 4.5
`)},
			expectedErr: "error unmarshaling JSON: while decoding JSON: unable to parse config; maxVersion must be in valid semver format if present: No Major.Minor.Patch elements found",
		},
		{
			name:     "Invalid/MajorVersionOnly",
			expected: ReleaseChannel{},
			args: args{data: []byte(`
maxVersion: 1
`)},
			expectedErr: "error unmarshaling JSON: while decoding JSON: unable to parse config; maxVersion must be in valid semver format if present: No Major.Minor.Patch elements found",
		},
		{
			name:     "Invalid/NonSemver",
			expected: ReleaseChannel{},
			args: args{data: []byte(`
minVersion: 1.2.3.4
`)},
			expectedErr: "error unmarshaling JSON: while decoding JSON: unable to parse config; minVersion must be in valid semver format if present: Invalid character(s) found in patch number \"3.4\"",
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
