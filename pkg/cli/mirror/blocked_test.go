package mirror

import (
	"testing"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
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
			blockedImages: []v1alpha2.Image{{Name: "openshift-migration-velero-restic-restore-helper-rhel8"}},
			ref:           "registry.redhat.io/rhmtc/openshift-migration-velero-restic-restore-helper-rhel8",
			want:          true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			img, err := imagesource.ParseReference(test.ref)
			require.NoError(t, err)

			actual, err := isBlocked(test.blockedImages, img.Ref)
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.want, actual)
			}

		})
	}
}
