package release

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"k8s.io/klog/v2"
)

const (
	// GraphMediaType is the media-type specified in the HTTP Accept header
	// of requests sent to the Cincinnati-v1 Graph API.
	GraphMediaType = "application/json"

	// Timeout when calling upstream Cincinnati stack.
	getUpdatesTimeout = time.Minute * 60
	// UpdateURL is the Cincinnati endpoint for the OpenShift platform.
	UpdateURL = "https://api.openshift.com/api/upgrades_info/v1/graph"
	// OkdUpdateURL is the Cincinnati endpoint for the OKD platform.
	OkdUpdateURL = "https://origin-release.ci.openshift.org/graph"

	ChannelInfo = "channel %q: %v"
)

// Error is returned when are unable to get updates.
type Error struct {
	// Reason is the reason suggested for the Cincinnati calculation error.
	Reason string

	// Message is the message suggested for Cincinnati calculation error..
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

// GetUpdates fetches the requested update payload from the specified
// upstream Cincinnati stack given the current version, architecture, and channel.
// The shortest path is calculated between the current and requested version from the graph edge
// data.
func GetUpdates(ctx context.Context, c Client, arch string, channel string, version semver.Version, reqVer semver.Version) (Update, Update, []Update, error) {
	var current Update
	var requested Update
	// Prepare parametrized cincinnati query.
	c.SetQueryParams(arch, channel, version.String())

	graph, err := getGraphData(ctx, c)
	if err != nil {
		return Update{}, Update{}, nil, &Error{
			Reason:  "APIRequestError",
			Message: fmt.Sprintf("version %s in channel %s: %v", version.String(), channel, err),
			cause:   err,
		}
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

	shortestPath := func(g map[int][]int, start, end int) []int {
		prev := map[int]int{}
		visited := map[int]struct{}{}
		queue := []int{start}
		visited[start] = struct{}{}
		prev[start] = -1

		for len(queue) > 0 {
			node := queue[0]
			queue = queue[1:]
			if node == end {
				break
			}

			for _, neighbor := range g[node] {
				if _, ok := visited[neighbor]; !ok {
					prev[neighbor] = node
					queue = append(queue, neighbor)
					visited[neighbor] = struct{}{}
				}
			}
		}

		// No path to end
		if _, ok := visited[end]; !ok {
			return []int{}
		}

		path := []int{end}
		for next := prev[end]; next != -1; next = prev[next] {
			path = append(path, next)
		}

		// Reverse path.
		for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
			path[i], path[j] = path[j], path[i]
		}

		return path
	}

	nextIdxs := shortestPath(edgesByOrigin, currentIdx, destinationIdx)

	var updates []Update
	for _, i := range nextIdxs {
		updates = append(updates, Update(graph.Nodes[i]))
	}

	return current, requested, updates, nil
}

// CalculateUpgrades fetches and calculates all the update payloads from the specified
// upstream Cincinnati stack given the current and target version and channel.
func CalculateUpgrades(ctx context.Context, c Client, arch, sourceChannel, targetChannel string, startVer, reqVer semver.Version) (Update, Update, []Update, error) {
	if sourceChannel == targetChannel {
		return GetUpdates(ctx, c, arch, targetChannel, startVer, reqVer)
	}

	// Check the major and minor versions are the same with different
	// channel prefixes
	source, target, _, err := getSemverFromChannels(sourceChannel, targetChannel)
	if err != nil {
		return Update{}, Update{}, nil, err
	}
	if source.EQ(target) {
		isBlocked, err := handleBlockedEdges(ctx, c, arch, targetChannel, startVer)
		if err != nil {
			return Update{}, Update{}, nil, err
		}
		if isBlocked {
			// If blocked path is found, just return the requested version and any accumulated
			// upgrades to the caller
			klog.Warningf("No upgrade path for %s in target channel %s", startVer.String(), targetChannel)
			return GetUpdates(ctx, c, arch, targetChannel, reqVer, reqVer)
		}
		return GetUpdates(ctx, c, arch, targetChannel, startVer, reqVer)
	}

	// Perform initial calculation for the source channel and
	// recurse through the rest until the target or a blocked
	// edge is hit.
	latest, err := GetChannelMinOrMax(ctx, c, arch, sourceChannel, false)
	if err != nil {
		return Update{}, Update{}, nil, fmt.Errorf(ChannelInfo, sourceChannel, err)
	}
	current, _, upgrades, err := GetUpdates(ctx, c, arch, sourceChannel, startVer, latest)
	if err != nil {
		return Update{}, Update{}, nil, fmt.Errorf(ChannelInfo, sourceChannel, err)
	}

	requested, newUpgrades, err := calculate(ctx, c, arch, sourceChannel, targetChannel, latest, reqVer)
	if err != nil {
		return Update{}, Update{}, nil, err
	}
	upgrades = append(upgrades, newUpgrades...)

	var finalUpgrades []Update
	seen := make(map[string]struct{}, len(upgrades))
	for _, upgrade := range upgrades {
		if _, ok := seen[upgrade.Image]; !ok {
			finalUpgrades = append(finalUpgrades, upgrade)
			seen[upgrade.Image] = struct{}{}
		}
	}

	return current, requested, finalUpgrades, nil
}

// calculate will calculate Cincinnati upgrades between channels by finding the latest versions in the source channels
// and incrementing the minor version until the target channel is reached.
func calculate(ctx context.Context, c Client, arch, sourceChannel, targetChannel string, startVer, reqVer semver.Version) (requested Update, upgrades []Update, err error) {
	source, target, prefix, err := getSemverFromChannels(sourceChannel, targetChannel)
	if err != nil {
		return requested, upgrades, err
	}
	// We immediately bump the source channel since current source channel upgrades have
	// already been calculated
	source.Minor++
	currChannel := fmt.Sprintf("%s-%v.%v", prefix, source.Major, source.Minor)

	var targetVer semver.Version
	if source.EQ(target) {
		// If this is the target channel major.minor get
		// requested version, so we don't exceed the maximum version
		// Set the target channel to make sure we have the intended
		// channel prefix
		targetVer = reqVer
		currChannel = targetChannel
	} else {
		targetVer, err = GetChannelMinOrMax(ctx, c, arch, currChannel, false)
		if err != nil {
			return requested, upgrades, err
		}
	}

	isBlocked, err := handleBlockedEdges(ctx, c, arch, currChannel, startVer)
	if err != nil {
		return requested, upgrades, err
	}
	if isBlocked {
		// If blocked path is found, just return the requested version and any accumulated
		// upgrades to the caller
		_, requested, _, err = GetUpdates(ctx, c, arch, targetChannel, targetVer, targetVer)
		//Warnf is 5?
		klog.Warningf("No upgrade path for %s in target channel %s", startVer.String(), targetChannel)
		return requested, upgrades, err
	}

	klog.V(1).Infof("Getting updates for version %s in channel %s", startVer.String(), currChannel)
	_, requested, upgrades, err = GetUpdates(ctx, c, arch, currChannel, startVer, targetVer)
	if err != nil {
		return requested, upgrades, err
	}

	if source.EQ(target) {
		return requested, upgrades, nil
	}

	currRequested, currUpgrades, err := calculate(ctx, c, arch, currChannel, targetChannel, targetVer, reqVer)
	if err != nil {
		return requested, upgrades, err
	}
	requested = currRequested
	upgrades = append(upgrades, currUpgrades...)

	return requested, upgrades, nil
}

// handleBlockedEdges will check for the starting version in the current channel
// if it does not exist the version is blocked.
func handleBlockedEdges(ctx context.Context, c Client, arch, targetChannel string, startVer semver.Version) (bool, error) {
	chanVersions, err := GetVersions(ctx, c, arch, targetChannel)
	if err != nil {
		return true, err
	}
	for _, v := range chanVersions {
		if v.EQ(startVer) {
			return false, nil
		}
	}
	return true, nil
}

// getSemverFromChannel will return the major and minor version from the source and target channels. The prefix returned is
// for the source channels for cross channel calculations.
func getSemverFromChannels(sourceChannel, targetChannel string) (source, target semver.Version, prefix string, err error) {
	// Get semver representation of source and target channel versions
	sourceIdx := strings.LastIndex(sourceChannel, "-")
	if sourceIdx == -1 {
		return source, target, prefix, fmt.Errorf("invalid channel name %s", sourceChannel)
	}
	targetIdx := strings.LastIndex(targetChannel, "-")
	if targetIdx == -1 {
		return source, target, prefix, fmt.Errorf("invalid channel name %s", targetChannel)
	}
	source, err = semver.Parse(fmt.Sprintf("%s.0", sourceChannel[sourceIdx+1:]))
	if err != nil {
		return source, target, prefix, err
	}
	target, err = semver.Parse(fmt.Sprintf("%s.0", targetChannel[targetIdx+1:]))
	if err != nil {
		return source, target, prefix, err
	}
	prefix = sourceChannel[:sourceIdx]
	return source, target, prefix, nil
}

// GetChannelMinOrMax fetches the minimum or maximum version from the specified
// upstream Cincinnati stack given architecture and channel.
func GetChannelMinOrMax(ctx context.Context, c Client, arch string, channel string, min bool) (semver.Version, error) {
	// Prepare parametrized cincinnati query.
	c.SetQueryParams(arch, channel, "")

	graph, err := getGraphData(ctx, c)
	if err != nil {
		return semver.Version{}, &Error{
			Reason:  "APIRequestError",
			Message: fmt.Sprintf(ChannelInfo, channel, err),
			cause:   err,
		}
	}

	// Find the all versions within the graph.
	var versionMatcher *regexp.Regexp
	if versionFilter := os.Getenv("VERSION_FILTER"); len(versionFilter) != 0 {
		klog.Info("Usage of the VERSION_FILTER environment variable is unsupported")
		versionMatcher, err = regexp.Compile(versionFilter)
		if err != nil {
			return semver.Version{}, &Error{
				Reason:  "InvalidVersionFilter",
				Message: fmt.Sprintf("Version filter '%s' is not a valid regular expression", versionFilter),
				cause:   err,
			}
		}
	}

	var Vers []semver.Version
	for _, node := range graph.Nodes {
		if versionMatcher == nil || versionMatcher.MatchString(node.Version.String()) {
			Vers = append(Vers, node.Version)
		}
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
// upstream Cincinnati stack.
func GetChannels(ctx context.Context, c Client, channel string) (map[string]struct{}, error) {
	// Prepare parametrized cincinnati query.
	c.SetQueryParams("", channel, "")

	graph, err := getGraphData(ctx, c)
	if err != nil {
		return nil, &Error{
			Reason:  "APIRequestError",
			Message: fmt.Sprintf(ChannelInfo, channel, err),
			cause:   err,
		}
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

// GetVersions will return all update payloads from the specified
// upstream Cincinnati stack given architecture and channel.
func GetVersions(ctx context.Context, c Client, arch, channel string) ([]semver.Version, error) {
	// Prepare parametrized cincinnati query.
	c.SetQueryParams(arch, channel, "")

	graph, err := getGraphData(ctx, c)
	if err != nil {
		return nil, &Error{
			Reason:  "APIRequestError",
			Message: fmt.Sprintf(ChannelInfo, channel, err),
			cause:   err,
		}
	}
	// Find the all versions within the graph.
	var Vers []semver.Version
	for _, node := range graph.Nodes {

		Vers = append(Vers, node.Version)
	}

	if len(Vers) == 0 {
		return nil, &Error{
			Reason:  "NoVersionsFound",
			Message: fmt.Sprintf("no cluster versions found in the %q channel", channel),
		}
	}

	semver.Sort(Vers)

	return Vers, nil
}

// GetUpdatesInRange will return all update payload within a semver range for a specified channel and architecture.
func GetUpdatesInRange(ctx context.Context, c Client, channel, arch string, updateRange semver.Range) ([]Update, error) {
	// Prepare parametrized cincinnati query.
	c.SetQueryParams(arch, channel, "")

	graph, err := getGraphData(ctx, c)
	if err != nil {
		return nil, &Error{
			Reason:  "APIRequestError",
			Message: fmt.Sprintf(ChannelInfo, channel, err),
			cause:   err,
		}
	}

	// Find the all updates within the range
	var updates []Update
	for _, node := range graph.Nodes {
		if updateRange(node.Version) {
			updates = append(updates, Update(node))
		}

	}
	return updates, nil
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
		} //else {
		//klog.V(5).Infof("Using a root CA pool with %n root CA subjects to request updates from %s", len(transport.TLSClientConfig.RootCAs.Subjects()), uri)
		//}
	}

	if transport != nil && transport.Proxy != nil {
		proxy, err := transport.Proxy(req)
		if err == nil && proxy != nil {
			klog.Infof("Using proxy %s to request updates from %s", proxy.Host, uri)
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
	body, err := io.ReadAll(resp.Body)
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
