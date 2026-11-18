// Package cincinnati contains functions for working with Cincinnati graph data.
package cincinnati

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/blang/semver/v4"
	"k8s.io/apimachinery/pkg/util/sets"
)

var ErrVersionNotFound = errors.New("node version not found")

type Graph struct {
	Nodes []node
	Edges [][]int
}

type node struct {
	Version  semver.Version    `json:"version"`
	Image    string            `json:"payload"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type Update node

func LoadGraphData(data []byte) (Graph, error) {
	var g Graph
	if err := json.Unmarshal(data, &g); err != nil {
		return g, fmt.Errorf("failed to load graph data: %w", err)
	}

	return g, nil
}

// GetVersions returns a sorted list of the all versions within the graph.
// The `versionMatcher` can be used to filter nodes.
func (o Graph) GetVersions(versionMatcher *regexp.Regexp) []semver.Version {
	vers := make([]semver.Version, 0, len(o.Nodes))
	for _, node := range o.Nodes {
		if versionMatcher == nil || versionMatcher.MatchString(node.Version.String()) {
			vers = append(vers, node.Version)
		}
	}
	semver.Sort(vers)

	return vers
}

const channelMetadataKey string = "io.openshift.upgrades.graph.release.channels"

func (o Graph) GetChannels() sets.Set[string] {
	channels := sets.New[string]()
	for _, node := range o.Nodes {
		if values, ok := node.Metadata[channelMetadataKey]; ok {
			for v := range strings.SplitSeq(values, ",") {
				channels.Insert(v)
			}
		}
	}
	return channels
}

func (o Graph) GetNodeByVersion(version semver.Version) (Update, int, error) {
	idx := slices.IndexFunc(o.Nodes, func(e node) bool { return version.EQ(e.Version) })
	if idx == -1 {
		return Update{}, idx, ErrVersionNotFound
	}
	return Update(o.Nodes[idx]), idx, nil
}

// MergeGraphs returns the union of the graphs. Each release appears once, identified by its
// version: when several graphs contain the same version, the release of the first of them is
// kept. The edges of every graph are kept between the merged releases, without duplicates, and
// no other edges are added. Edges referring to missing nodes are ignored.
func MergeGraphs(graphs ...Graph) Graph {
	var merged Graph
	indexByVersion := map[string]int{}
	seenEdges := map[[2]int]struct{}{}
	for _, graph := range graphs {
		indexes := make([]int, len(graph.Nodes))
		for i, n := range graph.Nodes {
			// Build metadata does not identify a version, as in GetNodeByVersion.
			version := n.Version
			version.Build = nil
			key := version.String()
			idx, ok := indexByVersion[key]
			if !ok {
				idx = len(merged.Nodes)
				indexByVersion[key] = idx
				merged.Nodes = append(merged.Nodes, n)
			}
			indexes[i] = idx
		}
		for _, edge := range graph.Edges {
			if !validEdge(edge, len(indexes)) {
				continue
			}
			mergedEdge := [2]int{indexes[edge[0]], indexes[edge[1]]}
			if _, ok := seenEdges[mergedEdge]; ok {
				continue
			}
			seenEdges[mergedEdge] = struct{}{}
			merged.Edges = append(merged.Edges, []int{mergedEdge[0], mergedEdge[1]})
		}
	}
	return merged
}

// validEdge returns whether the edge connects two of the nodes of a graph with the given number
// of nodes.
func validEdge(edge []int, nodes int) bool {
	return len(edge) == 2 && edge[0] >= 0 && edge[0] < nodes && edge[1] >= 0 && edge[1] < nodes
}
