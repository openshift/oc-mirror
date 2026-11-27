package cincinnati

import (
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestNodeUnmarshalJSON(t *testing.T) {
	tests := []struct {
		raw []byte
		exp node
		err string
	}{{
		raw: []byte(`{
			"version": "4.0.0-5",
			"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-5",
			"metadata": {}
		  }`),

		exp: node{
			Version:  semver.MustParse("4.0.0-5"),
			Image:    "quay.io/openshift-release-dev/ocp-release:4.0.0-5",
			Metadata: map[string]string{},
		},
	}, {
		raw: []byte(`{
			"version": "4.0.0-0.1",
			"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.1",
			"metadata": {
			  "description": "This is the beta1 image based on the 4.0.0-0.nightly-2019-01-15-010905 build"
			}
		  }`),
		exp: node{
			Version: semver.MustParse("4.0.0-0.1"),
			Image:   "quay.io/openshift-release-dev/ocp-release:4.0.0-0.1",
			Metadata: map[string]string{
				"description": "This is the beta1 image based on the 4.0.0-0.nightly-2019-01-15-010905 build",
			},
		},
	}, {
		raw: []byte(`{
			"version": "v4.0.0-0.1",
			"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.1",
			"metadata": {
			  "description": "This is the beta1 image based on the 4.0.0-0.nightly-2019-01-15-010905 build"
			}
		  }`),
		err: `Invalid character(s) found in major number "v4"`,
	}, {
		raw: []byte(`{
			"version": "4-0-0+0.1",
			"payload": "quay.io/openshift-release-dev/ocp-release:4.0.0-0.1",
			"metadata": {
			  "description": "This is the beta1 image based on the 4.0.0-0.nightly-2019-01-15-010905 build"
			}
		  }
	  `),

		err: "No Major.Minor.Patch elements found",
	}}

	for idx, test := range tests {
		t.Run(fmt.Sprintf("#%d", idx), func(t *testing.T) {
			var n node
			err := json.Unmarshal(test.raw, &n)
			if test.err == "" {
				require.NoError(t, err)
				require.Equal(t, test.exp, n)

			} else {
				require.EqualError(t, err, test.err)
			}
		})
	}
}

func TestGetVersions(t *testing.T) {
	ver4190 := semver.MustParse("4.19.0")
	ver4200 := semver.MustParse("4.20.0")
	ver4201 := semver.MustParse("4.20.1")
	ver4202 := semver.MustParse("4.20.2")
	ver42012 := semver.MustParse("4.20.12")

	t.Run("should get 0 results when graph data is empty", func(t *testing.T) {
		g := Graph{
			Nodes: []node{},
			Edges: [][]int{},
		}
		vers := g.GetVersions(nil)
		assert.Len(t, vers, 0)
	})
	t.Run("should return all versions when versionMatcher not specified", func(t *testing.T) {
		g := Graph{
			Nodes: []node{
				{Version: ver4201},
				{Version: ver4200},
			},
			Edges: [][]int{},
		}
		vers := g.GetVersions(nil)
		assert.Len(t, vers, 2)
		assert.EqualValues(t, []semver.Version{ver4200, ver4201}, vers, "sorted output")
	})
	t.Run("should filter versions when versionMatcher is specified", func(t *testing.T) {
		g := Graph{
			Nodes: []node{
				{Version: ver42012},
				{Version: ver4202},
				{Version: ver4201},
				{Version: ver4200},
				{Version: ver4202},
				{Version: ver4190},
			},
			Edges: [][]int{},
		}
		matcher := regexp.MustCompile(`4\.20\.1\d?`)
		vers := g.GetVersions(matcher)
		assert.EqualValues(t, []semver.Version{ver4201, ver42012}, vers)
	})
}

func TestGetChannels(t *testing.T) {
	ver4190 := semver.MustParse("4.19.0")
	ver4200 := semver.MustParse("4.20.0")
	ver4201 := semver.MustParse("4.20.1")

	t.Run("should get 0 results when graph data is empty", func(t *testing.T) {
		g := Graph{
			Nodes: []node{},
			Edges: [][]int{},
		}
		chs := g.GetChannels()
		assert.Len(t, chs, 0)
	})
	t.Run("should get 0 results when channel metadata not present", func(t *testing.T) {
		g := Graph{
			Nodes: []node{
				{Version: ver4190},
			},
			Edges: [][]int{},
		}
		chs := g.GetChannels()
		assert.Len(t, chs, 0)
	})
	t.Run("should return all channels in nodes' metadata", func(t *testing.T) {
		g := Graph{
			Nodes: []node{
				{Version: ver4190, Metadata: map[string]string{channelMetadataKey: "stable-4.19"}},
				{Version: ver4200, Metadata: map[string]string{channelMetadataKey: "stable-4.20"}},
				{Version: ver4201, Metadata: map[string]string{channelMetadataKey: "fast-4.20"}},
			},
			Edges: [][]int{},
		}
		chs := g.GetChannels()
		assert.EqualValues(t, []string{"fast-4.20", "stable-4.19", "stable-4.20"}, sets.List(chs))
	})
}

func TestGetNodeByVersion(t *testing.T) {
	t.Run("should fail when node not found", func(t *testing.T) {
		g := Graph{
			Nodes: []node{},
			Edges: [][]int{},
		}
		_, idx, err := g.GetNodeByVersion(semver.MustParse("4.21.0"))
		assert.ErrorIs(t, err, ErrVersionNotFound)
		assert.Equal(t, -1, idx)
	})
	t.Run("should succeed when node exists", func(t *testing.T) {
		ver4190 := semver.MustParse("4.19.0")
		ver4200 := semver.MustParse("4.20.0")
		ver4201 := semver.MustParse("4.20.1")
		g := Graph{
			Nodes: []node{{Version: ver4190}, {Version: ver4200}, {Version: ver4201}},
			Edges: [][]int{},
		}
		_, idx, err := g.GetNodeByVersion(ver4200)
		assert.NoError(t, err)
		assert.Equal(t, 1, idx)
	})
}

func TestMergeGraphs(t *testing.T) {
	n := func(version string) node {
		return node{Version: semver.MustParse(version), Image: "quay.io/openshift-release-dev/ocp-release:" + version}
	}

	t.Run("should deduplicate releases and keep the edges of every graph", func(t *testing.T) {
		target := Graph{
			Nodes: []node{n("4.22.1"), n("4.22.5"), n("5.0.0")},
			Edges: [][]int{{1, 2}},
		}
		source := Graph{
			Nodes: []node{n("4.22.5"), n("4.22.3"), n("4.22.1")},
			Edges: [][]int{{2, 1}, {1, 0}, {1, 0}},
		}
		merged := MergeGraphs(target, source)
		assert.Equal(t, []node{n("4.22.1"), n("4.22.5"), n("5.0.0"), n("4.22.3")}, merged.Nodes)
		// 4.22.5->5.0.0 (target), then 4.22.1->4.22.3 and 4.22.3->4.22.5 (source) once.
		assert.Equal(t, [][]int{{1, 2}, {0, 3}, {3, 1}}, merged.Edges)
	})

	t.Run("should not add edges between releases of different graphs", func(t *testing.T) {
		merged := MergeGraphs(
			Graph{Nodes: []node{n("4.21.1"), n("4.21.9")}, Edges: [][]int{{0, 1}}},
			Graph{Nodes: []node{n("4.22.2"), n("5.0.0")}, Edges: [][]int{{0, 1}}},
		)
		assert.Equal(t, []node{n("4.21.1"), n("4.21.9"), n("4.22.2"), n("5.0.0")}, merged.Nodes)
		assert.Equal(t, [][]int{{0, 1}, {2, 3}}, merged.Edges)
	})

	t.Run("should keep the release of the first graph and ignore invalid edges", func(t *testing.T) {
		first := node{Version: semver.MustParse("5.0.0"), Image: "first"}
		second := node{Version: semver.MustParse("5.0.0+build"), Image: "second"}
		merged := MergeGraphs(
			Graph{Nodes: []node{first}},
			Graph{Nodes: []node{second, n("4.22.1")}, Edges: [][]int{{1, 0}, {0, 2}, {-1, 0}, {0}}},
		)
		assert.Equal(t, []node{first, n("4.22.1")}, merged.Nodes)
		assert.Equal(t, [][]int{{1, 0}}, merged.Edges)
	})

	t.Run("should return an empty graph without graphs", func(t *testing.T) {
		assert.Equal(t, Graph{}, MergeGraphs())
	})
}
