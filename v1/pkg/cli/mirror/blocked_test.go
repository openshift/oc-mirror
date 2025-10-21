package mirror

import (
	"testing"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/stretchr/testify/require"
)

func TestImageBlocking(t *testing.T) {
	tests := []struct {
		name          string
		blockedImages []v1alpha2.Image
		ref           string
		want          bool
		err           string
	}{
		{
			name:          "Success/ImageBlocked",
			blockedImages: []v1alpha2.Image{{Name: "alpine"}},
			ref:           "docker.io/library/alpine:latest",
			want:          true,
		},
		{
			name:          "Success/ImageNotBlocked",
			blockedImages: []v1alpha2.Image{{Name: "alpine"}},
			ref:           "registry.redhat.io/ubi8/ubi:latest",
			want:          false,
		},
		{
			name:          "Success/ImageNotBlockedContainsKeyword",
			blockedImages: []v1alpha2.Image{{Name: "^alpine"}},
			ref:           "docker.io/library/notalpine:latest",
			want:          false,
		},
		{
			name:          "Success/ImageBlockedWildCard",
			blockedImages: []v1alpha2.Image{{Name: "ub*"}},
			ref:           "registry.redhat.io/ubi8/ubi:latest",
			want:          true,
		},
		{
			name:          "Success/ImageBlockedNoTag",
			blockedImages: []v1alpha2.Image{{Name: "registry.redhat.io/rhmtc/openshift-migration-velero-restic-restore-helper-rhel8:latest"}},
			ref:           "registry.redhat.io/rhmtc/openshift-migration-velero-restic-restore-helper-rhel8:latest",
			want:          true,
		},
		{
			name:          "Failure/InvalidRegexp",
			blockedImages: []v1alpha2.Image{{Name: "a(b"}},
			ref:           "registry.redhat.io/rhmtc/openshift-migration-velero-restic-restore-helper-rhel8",
			err:           "error parsing blocked image regular expression a(b: error parsing regexp: missing closing ): `a(b`",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := isBlocked(test.blockedImages, test.ref)
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.want, actual)
			}

		})
	}
}

func BenchmarkIsBlocked_1(b *testing.B) {
	blocked := []v1alpha2.Image{
		{Name: "alpine1"},
	}
	for i := 0; i < b.N; i++ {
		_, err := isBlocked(blocked, "test-registry.com/library/nomatch")
		require.NoError(b, err)
	}
}

func BenchmarkIsBlocked_5(b *testing.B) {
	blocked := []v1alpha2.Image{
		{Name: "alpine1"},
		{Name: "alpine2"},
		{Name: "alpine3"},
		{Name: "alpine4"},
		{Name: "alpine5"},
	}
	for i := 0; i < b.N; i++ {
		_, err := isBlocked(blocked, "test-registry.com/library/nomatch")
		require.NoError(b, err)
	}
}

func BenchmarkIsBlocked_10(b *testing.B) {
	blocked := []v1alpha2.Image{
		{Name: "alpine1"},
		{Name: "alpine2"},
		{Name: "alpine3"},
		{Name: "alpine4"},
		{Name: "alpine5"},
		{Name: "alpine6"},
		{Name: "alpine7"},
		{Name: "alpine8"},
		{Name: "alpine9"},
		{Name: "alpine10"},
	}
	for i := 0; i < b.N; i++ {
		_, err := isBlocked(blocked, "test-registry.com/library/nomatch")
		require.NoError(b, err)
	}

}
