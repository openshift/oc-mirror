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
