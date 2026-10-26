package delete

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/batch"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type recordingDeleteBatch struct {
	batch.BatchInterface
	images []v2alpha1.CopyImageSchema
}

func (b *recordingDeleteBatch) Worker(_ context.Context, cs v2alpha1.CollectorSchema, _ mirror.CopyOptions) (v2alpha1.CollectorSchema, error) {
	b.images = append(b.images, cs.AllImages...)
	return cs, nil
}

func TestDeleteReleaseRepositories(t *testing.T) {
	for _, imageType := range []v2alpha1.ImageType{v2alpha1.TypeOCPRelease, v2alpha1.TypeOCPReleaseContent} {
		for _, tc := range []struct {
			name, ref string
			valid     bool
		}{
			{"custom repository", "mirror.example.com/team/openshift4/ocp-release:v1", true},
			{"default payload repository", "mirror.example.com/team/openshift/release-images:v1", true},
			{"default component repository", "mirror.example.com/team/openshift/release:v1-component", true},
			{"different registry", "other.example.com/team/openshift4/ocp-release:v1", false},
			{"different namespace", "mirror.example.com/other/openshift4/ocp-release:v1", false},
			{"namespace prefix collision", "mirror.example.com/team-other/openshift4/ocp-release:v1", false},
		} {
			t.Run(tc.name+"/"+imageType.String(), func(t *testing.T) {
				worker := &recordingDeleteBatch{}
				d := DeleteImages{
					Log:              clog.New("error"),
					Batch:            worker,
					LocalStorageFQDN: "localhost:55000",
					Opts: mirror.CopyOptions{Global: &mirror.GlobalOptions{
						DeleteDestination: "docker://mirror.example.com/team",
						ForceCacheDelete:  true,
					}},
				}
				list := v2alpha1.DeleteImageList{Items: []v2alpha1.DeleteItem{
					{ImageName: "quay.io/source/release:v1", ImageReference: "docker://" + tc.ref, Type: imageType},
				}}
				err := d.DeleteRegistryImages(list)
				if !tc.valid {
					require.ErrorContains(t, err, "does not match")
					require.Empty(t, worker.images, "out-of-scope images must never reach the delete worker")
					return
				}
				require.NoError(t, err)
				require.Len(t, worker.images, 2)
				require.Equal(t, "docker://"+tc.ref, worker.images[0].Destination)
				require.Equal(t, "docker://localhost:55000/"+strings.TrimPrefix(tc.ref, "mirror.example.com/team/"), worker.images[1].Destination)
			})
		}
	}
}
