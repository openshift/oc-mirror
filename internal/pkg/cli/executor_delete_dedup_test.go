package cli

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/delete"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

const (
	dedupDestination = "docker://localhost:5000/myregistry"
	dedupCacheFQDN   = "localhost:55000"
	dedupDigestA     = "sha256:44d75007b39e0e1bbf1bcfd0721245add54c54c3f83903f8926fb4bef6827aa2"
	dedupDigestB     = "sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea"
)

// dedupTagDigestEntry mirrors the additional images collector output in delete mode for
// repo:tag@digest: the cache is keyed by digest, the destination by the human tag.
func dedupTagDigestEntry(tag, digestTag, origin string) v2alpha1.CopyImageSchema {
	return v2alpha1.CopyImageSchema{
		Source:      "docker://" + dedupCacheFQDN + "/testns/repo:" + digestTag,
		Destination: dedupDestination + "/testns/repo:" + tag,
		Origin:      origin,
		Type:        v2alpha1.TypeGeneric,
	}
}

func TestRemoveDuplicatedImagesDeleteMode(t *testing.T) {
	entryA := dedupTagDigestEntry("v1", "sha256-a", "test.registry.io/testns/repo:v1@"+dedupDigestA)
	entryB := dedupTagDigestEntry("v1", "sha256-b", "test.registry.io/testns/repo:v1@"+dedupDigestB)
	// --delete-v1-images: digest only images are all addressed as :latest at the destination
	latestA := dedupTagDigestEntry("latest", "sha256-a", "test.registry.io/testns/repo@"+dedupDigestA)
	latestB := dedupTagDigestEntry("latest", "sha256-b", "test.registry.io/testns/repo@"+dedupDigestB)

	tests := []struct {
		name     string
		mode     mirror.Mode
		images   []v2alpha1.CopyImageSchema
		expected []v2alpha1.CopyImageSchema
	}{
		{
			name:     "delete keeps adjacent entries with distinct origins sharing a destination",
			mode:     mirror.DeleteMode,
			images:   []v2alpha1.CopyImageSchema{entryA, entryB},
			expected: []v2alpha1.CopyImageSchema{entryA, entryB},
		},
		{
			name:     "delete keeps distinct v1 digests sharing latest",
			mode:     mirror.DeleteMode,
			images:   []v2alpha1.CopyImageSchema{latestA, latestB},
			expected: []v2alpha1.CopyImageSchema{latestA, latestB},
		},
		{
			name:     "delete compacts exact repeats",
			mode:     mirror.DeleteMode,
			images:   []v2alpha1.CopyImageSchema{entryA, entryA, entryB, entryB},
			expected: []v2alpha1.CopyImageSchema{entryA, entryB},
		},
		{
			name:     "copy compacts exact repeats",
			mode:     mirror.CopyMode,
			images:   []v2alpha1.CopyImageSchema{entryA, entryA},
			expected: []v2alpha1.CopyImageSchema{entryA},
		},
		{
			name:     "copy keeps entries with distinct origins",
			mode:     mirror.CopyMode,
			images:   []v2alpha1.CopyImageSchema{entryA, entryB},
			expected: []v2alpha1.CopyImageSchema{entryA, entryB},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			images := append([]v2alpha1.CopyImageSchema{}, tt.images...)
			assert.Equal(t, tt.expected, removeDuplicatedImages(images, string(tt.mode)))
		})
	}
}

// dedupStaticCollector returns fixed collector results so CollectAll can be exercised in
// delete mode without registries.
type dedupStaticCollector struct {
	additional []v2alpha1.CopyImageSchema
}

func (c *dedupStaticCollector) ReleaseImageCollector(context.Context) (v2alpha1.CollectorSchema, error) {
	return v2alpha1.CollectorSchema{}, nil
}

func (c *dedupStaticCollector) GraphImage() (string, error) {
	return "", errors.New("no graph image")
}

func (c *dedupStaticCollector) ReleaseImage(context.Context) (string, error) {
	return "", errors.New("no release image")
}

func (c *dedupStaticCollector) OperatorImageCollector(context.Context) (v2alpha1.CollectorSchema, error) {
	return v2alpha1.CollectorSchema{}, nil
}

func (c *dedupStaticCollector) AdditionalImagesCollector(context.Context) (v2alpha1.CollectorSchema, error) {
	return v2alpha1.CollectorSchema{AllImages: append([]v2alpha1.CopyImageSchema{}, c.additional...)}, nil
}

func (c *dedupStaticCollector) HelmImageCollector(context.Context) (v2alpha1.CollectorSchema, error) {
	return v2alpha1.CollectorSchema{}, nil
}

// TestCollectAllDeleteModeRetainsDistinctOrigins drives the delete --generate consumer path:
// CollectAll (per collector compaction) feeds the delete package, which writes the plan.
func TestCollectAllDeleteModeRetainsDistinctOrigins(t *testing.T) {
	tests := []struct {
		name     string
		images   []v2alpha1.CopyImageSchema
		expected []string
	}{
		{
			name: "tag and digest images sharing a tag",
			images: []v2alpha1.CopyImageSchema{
				dedupTagDigestEntry("v1", "sha256-44d75007b39e0e1bbf1bcfd0721245add54c54c3f83903f8926fb4bef6827aa2", "test.registry.io/testns/repo:v1@"+dedupDigestA),
				dedupTagDigestEntry("v1", "sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", "test.registry.io/testns/repo:v1@"+dedupDigestB),
			},
			expected: []string{dedupDestination + "/testns/repo@" + dedupDigestA, dedupDestination + "/testns/repo@" + dedupDigestB},
		},
		{
			name: "delete-v1-images digest only images sharing latest",
			images: []v2alpha1.CopyImageSchema{
				dedupTagDigestEntry("latest", "sha256-44d75007b39e0e1bbf1bcfd0721245add54c54c3f83903f8926fb4bef6827aa2", "test.registry.io/testns/repo@"+dedupDigestA),
				dedupTagDigestEntry("latest", "sha256-f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e41baebea", "test.registry.io/testns/repo@"+dedupDigestB),
			},
			expected: []string{dedupDestination + "/testns/repo@" + dedupDigestA, dedupDestination + "/testns/repo@" + dedupDigestB},
		},
		{
			name: "exact repeats are still written once",
			images: []v2alpha1.CopyImageSchema{
				dedupTagDigestEntry("v1", "sha256-44d75007b39e0e1bbf1bcfd0721245add54c54c3f83903f8926fb4bef6827aa2", "test.registry.io/testns/repo:v1@"+dedupDigestA),
				dedupTagDigestEntry("v1", "sha256-44d75007b39e0e1bbf1bcfd0721245add54c54c3f83903f8926fb4bef6827aa2", "test.registry.io/testns/repo:v1@"+dedupDigestA),
			},
			expected: []string{dedupDestination + "/testns/repo:v1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := clog.New("trace")
			opts := &mirror.CopyOptions{
				Global: &mirror.GlobalOptions{
					WorkingDir:        t.TempDir(),
					DeleteDestination: dedupDestination,
				},
				Destination:      dedupDestination,
				Mode:             mirror.DiskToMirror,
				Function:         string(mirror.DeleteMode),
				LocalStorageFQDN: dedupCacheFQDN,
			}
			collector := &dedupStaticCollector{additional: tt.images}
			ex := &ExecutorSchema{
				Log:              log,
				Opts:             opts,
				Release:          collector,
				Operator:         collector,
				AdditionalImages: collector,
				HelmCollector:    collector,
			}

			collected, err := ex.CollectAll(context.Background())
			require.NoError(t, err)

			deleter := delete.New(log, *opts, nil, nil, v2alpha1.ImageSetConfiguration{}, nil, "/tmp", nil)
			require.NoError(t, deleter.WriteDeleteMetaData(context.Background(), collected.AllImages))
			plan, err := deleter.ReadDeleteMetaData()
			require.NoError(t, err)

			refs := make([]string, 0, len(plan.Items))
			for _, item := range plan.Items {
				refs = append(refs, item.ImageReference)
			}
			sort.Strings(refs)
			assert.Equal(t, tt.expected, refs)
		})
	}
}
