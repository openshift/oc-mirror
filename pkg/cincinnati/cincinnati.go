package cincinnati

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
	"k8s.io/klog/v2"
)

// Copied from https://github.com/openshift/cluster-version-operator/blob/release-4.9/pkg/cincinnati/cincinnati.go

const (
	// GraphMediaType is the media-type specified in the HTTP Accept header
	// of requests sent to the Cincinnati-v1 Graph API.
	GraphMediaType = "application/json"

	// Timeout when calling upstream Cincinnati stack.
	getUpdatesTimeout = time.Minute * 60
	// Update urls
	UpdateUrl    = "https://api.openshift.com/api/upgrades_info/v1/graph"
	OkdUpdateURL = "https://origin-release.ci.openshift.org/graph"
	OkdChannel   = "okd"
)

// Client is a Cincinnati client which can be used to fetch update graphs from
// an upstream Cincinnati stack.

// Error is returned when are unable to get updates.
type Error struct {
	// Reason is the reason suggested for the ClusterOperator status condition.
	Reason string

	// Message is the message suggested for the ClusterOperator status condition.
	Message string

	// cause is the upstream error, if any, being wrapped by this error.
	cause error
}

// Error serializes the error as a string, to satisfy the error interface.
func (err *Error) Error() string {
	return fmt.Sprintf("%s: %s", err.Reason, err.Message)
}

// Update is a single node from the update graph.
type Update node

// GetUpdates fetches the current and requested (if applicable) update payload from the specified
// upstream Cincinnati stack given the current version and channel. The next-
// applicable updates are determined by downloading the update graph, finding
// the current version within that graph (typically the root node), and then
// finding all of the children. These children are the available updates for
// the current version and their payloads indicate from where the actual update
// image can be downloaded.
func GetUpdates(ctx context.Context, c Client, arch string, channel string, version semver.Version, reqVer semver.Version) (Update, Update, []Update, error) {
	var current Update
	var requested Update
	// Prepare parametrized cincinnati query.
	c.SetQueryParams(arch, channel, version.String())

	graph, err := getGraphData(ctx, c)
	if err != nil {
		return Update{}, Update{}, nil, fmt.Errorf("error getting graph data for version %s in channel %s", version.String(), channel)
	}

	// Find the current version within the graph.
	var currentIdx int
	found := false
	for i, node := range graph.Nodes {
		if version.EQ(node.Version) {
			currentIdx = i
			current = Update(graph.Nodes[i])
			found = true
			break
		}
	}
	if !found {
		return current, requested, nil, &Error{
			Reason:  "VersionNotFound",
			Message: fmt.Sprintf("current version %s not found in the %q channel", version, channel),
		}
	}

	var destinationIdx int
	found = false
	for i, node := range graph.Nodes {
		if reqVer.EQ(node.Version) {
			destinationIdx = i
			requested = Update(graph.Nodes[i])
			found = true
			break
		}
	}
	if !found {
		return current, requested, nil, &Error{
			Reason:  "VersionNotFound",
			Message: fmt.Sprintf("requested version %s not found in the %q channel", reqVer, channel),
		}
	}

	edgesByOrigin := make(map[int][]int, len(graph.Nodes))
	for _, edge := range graph.Edges {
		edgesByOrigin[edge.Origin] = append(edgesByOrigin[edge.Origin], edge.Destination)
	}

	// Sort destination by semver to ensure deterministic result
	for origin, destinations := range edgesByOrigin {
		sort.Slice(destinations, func(i, j int) bool {
			return graph.Nodes[destinations[i]].Version.GT(graph.Nodes[destinations[j]].Version)
		})
		edgesByOrigin[origin] = destinations
	}

	var shortestPath func(map[int][]int, int, int, path) []int
	shortestPath = func(g map[int][]int, start, end int, path path) []int {
		path = append(path, start)
		if start == end {
			return path
		}
		adj := g[start]
		// If we get through the map and the start never
		// reaches the end, return nothing
		if len(adj) == 0 {
			return []int{}
		}
		shortest := make([]int, 0)
		for _, node := range adj {
			if !path.has(node) {
				currPath := shortestPath(g, node, end, path)
				if len(currPath) > 0 {
					if len(shortest) == 0 || len(currPath) < len(shortest) {
						shortest = currPath
					}
				}
			}
		}
		return shortest
	}

	nextIdxs := shortestPath(edgesByOrigin, currentIdx, destinationIdx, path{})

	var updates []Update
	for _, i := range nextIdxs {
		updates = append(updates, Update(graph.Nodes[i]))
	}

	return current, requested, updates, nil
}

// CalculateUpgrades fetches and calculates all the update payloads from the specified
// upstream Cincinnati stack given the current and target version and channel
func CalculateUpgrades(ctx context.Context, c Client, arch, sourceChannel, targetChannel string, startVer, reqVer semver.Version) (Update, Update, []Update, error) {
	if sourceChannel == targetChannel {
		return GetUpdates(ctx, c, arch, targetChannel, startVer, reqVer)
	}

	// Perform initial calculation for the source channel and
	// recurse through the rest until the target or a blocked
	// edge is hit
	latest, err := GetChannelMinorMax(ctx, c, arch, sourceChannel, false)
	if err != nil {
		return Update{}, Update{}, nil, fmt.Errorf("cannot get latest: %v", err)
	}
	current, _, upgrades, err := GetUpdates(ctx, c, arch, sourceChannel, startVer, latest)
	if err != nil {
		return Update{}, Update{}, nil, fmt.Errorf("cannot get current: %v", err)
	}

	requested, newUpgrades, err := calculate(ctx, c, arch, sourceChannel, targetChannel, latest, reqVer)
	upgrades = append(upgrades, newUpgrades...)

	var finalUpgrades []Update
	seen := make(map[string]struct{}, len(upgrades))
	for _, upgrade := range upgrades {
		if _, ok := seen[upgrade.Image]; !ok {
			finalUpgrades = append(finalUpgrades, upgrade)
			seen[upgrade.Image] = struct{}{}
		}
	}

	return current, requested, finalUpgrades, err
}

func calculate(ctx context.Context, c Client, arch, sourceChannel, targetChannel string, startVer, reqVer semver.Version) (requested Update, upgrades []Update, err error) {
	// Get semver representation of source and target channel versions
	sourceIdx := strings.LastIndex(sourceChannel, "-")
	if sourceIdx == -1 {
		return requested, upgrades, fmt.Errorf("invalid channel name %s", sourceChannel)
	}
	targetIdx := strings.LastIndex(targetChannel, "-")
	if targetIdx == -1 {
		return requested, upgrades, fmt.Errorf("invalid channel name %s", targetChannel)
	}
	source := semver.MustParse(fmt.Sprintf("%s.0", sourceChannel[sourceIdx+1:]))
	target := semver.MustParse(fmt.Sprintf("%s.0", targetChannel[targetIdx+1:]))

	// We immediately bump the source channel since current source channel upgrades have
	// already been calculated
	source.Minor++
	currChannel := fmt.Sprintf("%s-%v.%v", sourceChannel[:sourceIdx], source.Major, source.Minor)

	var targetVer semver.Version
	if currChannel == targetChannel {
		// If this is the target channel get
		// requested version so we don't exceed the maximun version
		targetVer = reqVer
		logrus.Info(targetVer)
	} else {
		targetVer, err = GetChannelMinOrMax(ctx, c, arch, currChannel, false)
		if err != nil {
			return requested, upgrades, nil
		}
	}

	// Handles blocked edges
	chanVersions, err := GetVersions(ctx, c, currChannel)
	if err != nil {
		return requested, upgrades, nil
	}
	foundVersions := make(map[string]struct{})
	for _, v := range chanVersions {
		foundVersions[v.String()] = struct{}{}
	}

	if _, found := foundVersions[startVer.String()]; !found {
		// If blocked path is found, just return the requested version and any accumulated
		// upgrades to the caller
		_, requested, _, err = GetUpdates(ctx, c, arch, targetChannel, targetVer, targetVer)
		logrus.Warnf("No upgrade path for %s in target channel %s", startVer.String(), targetChannel)
		return requested, upgrades, err
	}

	logrus.Debugf("Getting updates for version %s in channel %s", startVer.String(), currChannel)
	_, requested, upgrades, err = GetUpdates(ctx, c, arch, currChannel, startVer, targetVer)
	if err != nil {
		return requested, upgrades, nil
	}

	if source.EQ(target) {
		return requested, upgrades, nil
	}

	req, up, err := calculate(ctx, c, arch, currChannel, targetChannel, targetVer, reqVer)
	if err != nil {
		return requested, upgrades, nil
	}
	requested = req
	upgrades = append(upgrades, up...)

	return requested, upgrades, nil
}

// GetChannelLatest fetches the latest version from the specified
// upstream Cincinnati stack given architecture and channel
func GetChannelMinOrMax(ctx context.Context, c client, arch string, channel string, min bool) (semver.Version, error) {
	// Prepare parametrized cincinnati query.
	c.SetQueryParams(arch, channel, "")

	graph, err := getGraphData(ctx, c)
	if err != nil {
		return semver.Version{}, fmt.Errorf("error getting graph data for channel %s", channel)
	}

	// Find the all versions within the graph.
	Vers := []semver.Version{}
	for _, node := range graph.Nodes {

		Vers = append(Vers, node.Version)

	}

	semver.Sort(Vers)

	if len(Vers) == 0 {
		return semver.Version{}, &Error{
			Reason:  "NoVersionsFound",
			Message: fmt.Sprintf("no cluster versions found for %q in the %q channel", arch, channel),
		}
	}

	if min {
		return Vers[0], nil
	}

	return Vers[len(Vers)-1], nil
}

// GetChannels fetches the channels containing update payloads from the specified
// upstream Cincinnati stack
func GetChannels(ctx context.Context, c Client, channel string) (map[string]struct{}, error) {
	// Prepare parametrized cincinnati query.
	c.SetQueryParams("", channel, "")

	graph, err := getGraphData(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("error getting graph data for channel %s", channel)
	}

	channels := make(map[string]struct{})

	for _, node := range graph.Nodes {
		values := node.Metadata["io.openshift.upgrades.graph.release.channels"]

		for _, value := range strings.Split(values, ",") {
			channels[value] = struct{}{}
		}
	}

	return channels, nil
}

// GetVersion will return all OCP/OKD versions in a specified channel fetches the current and requested (if applicable) update payload from the specified
// upstream Cincinnati stack given the current version and channel
func GetVersions(ctx context.Context, c Client, channel string) ([]semver.Version, error) {
	// Prepare parametrized cincinnati query.
	c.SetQueryParams("", channel, "")

	graph, err := getGraphData(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("error getting graph data for channel %s", channel)
	}
	// Find the all versions within the graph.
	Vers := []semver.Version{}
	for _, node := range graph.Nodes {

		Vers = append(Vers, node.Version)

	}

	semver.Sort(Vers)

	if len(Vers) == 0 {
		return nil, &Error{
			Reason:  "NoVersionsFound",
			Message: fmt.Sprintf("no cluster versions found in the %q channel", channel),
		}
	}

	return Vers, nil
}

// getGraphData fetches the update graph from the upstream Cincinnati stack given the current version and channel
func getGraphData(ctx context.Context, c Client) (graph graph, err error) {
	transport := c.GetTransport()
	uri := c.GetURL()
	// Download the update graph.
	req, err := http.NewRequest("GET", uri.String(), nil)
	if err != nil {
		return graph, &Error{Reason: "InvalidRequest", Message: err.Error(), cause: err}
	}
	req.Header.Add("Accept", GraphMediaType)
	if transport != nil && transport.TLSClientConfig != nil {
		if c.GetTransport().TLSClientConfig.ClientCAs == nil {
			klog.V(5).Infof("Using a root CA pool with 0 root CA subjects to request updates from %s", uri)
		} else {
			klog.V(5).Infof("Using a root CA pool with %n root CA subjects to request updates from %s", len(transport.TLSClientConfig.RootCAs.Subjects()), uri)
		}
	}

	if transport != nil && transport.Proxy != nil {
		proxy, err := transport.Proxy(req)
		if err == nil && proxy != nil {
			klog.V(5).Infof("Using proxy %s to request updates from %s", proxy.Host, uri)
		}
	}

	client := http.Client{}
	if transport != nil {
		client.Transport = transport
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, getUpdatesTimeout)
	defer cancel()
	resp, err := client.Do(req.WithContext(timeoutCtx))
	if err != nil {
		return graph, &Error{Reason: "RemoteFailed", Message: err.Error(), cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return graph, &Error{Reason: "ResponseFailed", Message: fmt.Sprintf("unexpected HTTP status: %s", resp.Status)}
	}

	// Parse the graph.
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return graph, &Error{Reason: "ResponseFailed", Message: err.Error(), cause: err}
	}

	if err = json.Unmarshal(body, &graph); err != nil {
		return graph, &Error{Reason: "ResponseInvalid", Message: err.Error(), cause: err}
	}

	return graph, nil
}

type graph struct {
	Nodes []node
	Edges []edge
}

type node struct {
	Version  semver.Version    `json:"version"`
	Image    string            `json:"payload"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type edge struct {
	Origin      int
	Destination int
}

// UnmarshalJSON unmarshals an edge in the update graph. The edge's JSON
// representation is a two-element array of indices, but Go's representation is
// a struct with two elements so this custom unmarshal method is required.
func (e *edge) UnmarshalJSON(data []byte) error {
	var fields []int
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	if len(fields) != 2 {
		return fmt.Errorf("expected 2 fields, found %d", len(fields))
	}

	e.Origin = fields[0]
	e.Destination = fields[1]

	return nil
}

type path []int

func (p path) has(num int) bool {
	for _, v := range p {
		if num == v {
			return true
		}
	}
	return false
}
