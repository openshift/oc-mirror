package release

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/blang/semver/v4"

	"github.com/openshift/oc-mirror/v2/internal/pkg/cincinnati"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

const ChannelInfo = "channel %q: %v"

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
func (o Error) Error() string {
	return fmt.Sprintf("%s: %s", o.Reason, o.Message)
}

// GetUpdates fetches the requested update payload from the specified
// upstream Cincinnati stack given the current version, architecture, and channel.
// The shortest path is calculated between the current and requested version from the graph edge
// data.
func GetUpdates(ctx context.Context, cs CincinnatiSchema, channel string, version semver.Version, reqVer semver.Version) (cincinnati.Update, cincinnati.Update, []cincinnati.Update, error) { //nolint:cyclop // FIXME: needs further refactoring.
	graph, err := getGraphData(ctx, cs, channel, version.String())
	if err != nil {
		return cincinnati.Update{}, cincinnati.Update{}, nil, &Error{
			Reason:  "APIRequestError",
			Message: fmt.Sprintf("version %s in channel %s: %v", version.String(), channel, err),
			cause:   err,
		}
	}

	var current cincinnati.Update
	var requested cincinnati.Update

	// Find the current version within the graph.
	current, currentIdx, err := graph.GetNodeByVersion(version)
	if errors.Is(err, cincinnati.ErrVersionNotFound) {
		return current, requested, nil, &Error{
			Reason:  "VersionNotFound",
			Message: fmt.Sprintf("current version %s not found in the %q channel", version, channel),
		}
	}

	requested, destinationIdx, err := graph.GetNodeByVersion(reqVer)
	if errors.Is(err, cincinnati.ErrVersionNotFound) {
		return current, requested, nil, &Error{
			Reason:  "VersionNotFound",
			Message: fmt.Sprintf("requested version %s not found in the %q channel", reqVer, channel),
		}
	}

	edgesByOrigin := make(map[int][]int, len(graph.Nodes))
	for _, edge := range graph.Edges {
		edgesByOrigin[edge[0]] = append(edgesByOrigin[edge[0]], edge[1])
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

	var updates []cincinnati.Update
	for _, i := range nextIdxs {
		updates = append(updates, cincinnati.Update(graph.Nodes[i]))
	}

	return current, requested, updates, nil
}

// CalculateUpgrades fetches and calculates all the update payloads from the specified
// upstream Cincinnati stack given the current and target version and channel.
func CalculateUpgrades(ctx context.Context, cs CincinnatiSchema, sourceChannel, targetChannel string, startVer, reqVer semver.Version) (cincinnati.Update, cincinnati.Update, []cincinnati.Update, error) { //nolint:cyclop // FIXME: needs further refactoring.
	if sourceChannel == targetChannel {
		return GetUpdates(ctx, cs, targetChannel, startVer, reqVer)
	}

	// Check the major and minor versions are the same with different
	// channel prefixes
	source, target, _, err := getSemverFromChannels(sourceChannel, targetChannel)
	if err != nil {
		return cincinnati.Update{}, cincinnati.Update{}, nil, err
	}
	if source.EQ(target) {
		isBlocked, err := handleBlockedEdges(ctx, cs, targetChannel, startVer)
		if err != nil {
			return cincinnati.Update{}, cincinnati.Update{}, nil, err
		}

		// If blocked path is found, just return the requested version and any accumulated
		// upgrades to the caller
		if isBlocked {
			cs.Log.Warn("No upgrade path for %s in target channel %s", startVer.String(), targetChannel)
			return GetUpdates(ctx, cs, targetChannel, reqVer, reqVer)
		}
		return GetUpdates(ctx, cs, targetChannel, startVer, reqVer)
	}

	// Perform initial calculation for the source channel and
	// recurse through the rest until the target or a blocked
	// edge is hit.
	latest, err := GetChannelMinOrMax(ctx, cs, sourceChannel, false)
	if err != nil {
		return cincinnati.Update{}, cincinnati.Update{}, nil, fmt.Errorf(ChannelInfo, sourceChannel, err)
	}
	current, _, upgrades, err := GetUpdates(ctx, cs, sourceChannel, startVer, latest)
	if err != nil {
		return cincinnati.Update{}, cincinnati.Update{}, nil, fmt.Errorf(ChannelInfo, sourceChannel, err)
	}

	requested, newUpgrades, err := calculate(ctx, cs, sourceChannel, targetChannel, latest, reqVer)
	if err != nil {
		return cincinnati.Update{}, cincinnati.Update{}, nil, err
	}
	upgrades = append(upgrades, newUpgrades...)

	var finalUpgrades []cincinnati.Update
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
func calculate(ctx context.Context, cs CincinnatiSchema, sourceChannel, targetChannel string, startVer, reqVer semver.Version) (requested cincinnati.Update, upgrades []cincinnati.Update, err error) {
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
		targetVer, err = GetChannelMinOrMax(ctx, cs, currChannel, false)
		if err != nil {
			return requested, upgrades, err
		}
	}

	isBlocked, err := handleBlockedEdges(ctx, cs, currChannel, startVer)
	if err != nil {
		return requested, upgrades, err
	}
	if isBlocked {
		// If blocked path is found, just return the requested version and any accumulated
		// upgrades to the caller
		_, requested, _, err = GetUpdates(ctx, cs, targetChannel, targetVer, targetVer)
		cs.Log.Warn("No upgrade path for %s in target channel %s", startVer.String(), targetChannel)
		return requested, upgrades, err
	}

	cs.Log.Debug("Getting updates for version %s in channel %s", startVer.String(), currChannel)
	_, requested, upgrades, err = GetUpdates(ctx, cs, currChannel, startVer, targetVer)
	if err != nil {
		return requested, upgrades, err
	}

	if source.EQ(target) {
		return requested, upgrades, nil
	}

	currRequested, currUpgrades, err := calculate(ctx, cs, currChannel, targetChannel, targetVer, reqVer)
	if err != nil {
		return requested, upgrades, err
	}
	requested = currRequested
	upgrades = append(upgrades, currUpgrades...)

	return requested, upgrades, nil
}

// handleBlockedEdges will check for the starting version in the current channel
// if it does not exist the version is blocked.
func handleBlockedEdges(ctx context.Context, cs CincinnatiSchema, targetChannel string, startVer semver.Version) (bool, error) {
	chanVersions, err := GetVersions(ctx, cs, targetChannel)
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
func GetChannelMinOrMax(ctx context.Context, cs CincinnatiSchema, channel string, min bool) (semver.Version, error) {
	graph, err := getGraphData(ctx, cs, channel, "")
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
		cs.Log.Info("Usage of the VERSION_FILTER environment variable is unsupported")
		versionMatcher, err = regexp.Compile(versionFilter)
		if err != nil {
			return semver.Version{}, &Error{
				Reason:  "InvalidVersionFilter",
				Message: fmt.Sprintf("Version filter '%s' is not a valid regular expression", versionFilter),
				cause:   err,
			}
		}
	}

	vers := graph.GetVersions(versionMatcher)
	if len(vers) == 0 {
		return semver.Version{}, &Error{
			Reason:  "NoVersionsFound",
			Message: fmt.Sprintf("no cluster versions found for %q in the %q channel", cs.CincinnatiParams.Arch, channel),
		}
	}

	if min {
		return vers[0], nil
	}

	return vers[len(vers)-1], nil
}

// GetVersions will return all update payloads from the specified
// upstream Cincinnati stack given architecture and channel.
func GetVersions(ctx context.Context, cs CincinnatiSchema, channel string) ([]semver.Version, error) {
	graph, err := getGraphData(ctx, cs, channel, "")
	if err != nil {
		return nil, &Error{
			Reason:  "APIRequestError",
			Message: fmt.Sprintf(ChannelInfo, channel, err),
			cause:   err,
		}
	}

	vers := graph.GetVersions(nil)
	if len(vers) == 0 {
		return nil, &Error{
			Reason:  "NoVersionsFound",
			Message: fmt.Sprintf("no cluster versions found in the %q channel", channel),
		}
	}

	return vers, nil
}

// GetUpdatesInRange will return all update payload within a semver range for a specified channel and architecture.
func GetUpdatesInRange(ctx context.Context, cs CincinnatiSchema, channel string, updateRange semver.Range) ([]cincinnati.Update, error) {
	graph, err := getGraphData(ctx, cs, channel, "")
	if err != nil {
		return nil, &Error{
			Reason:  "APIRequestError",
			Message: fmt.Sprintf(ChannelInfo, channel, err),
			cause:   err,
		}
	}

	// Find the all updates within the range
	var updates []cincinnati.Update
	for _, node := range graph.Nodes {
		if updateRange(node.Version) {
			updates = append(updates, cincinnati.Update(node))
		}
	}
	return updates, nil
}

// getGraphData fetches the update graph from the upstream Cincinnati stack given the current version and channel
func getGraphData(ctx context.Context, cs CincinnatiSchema, channel string, version string) (graph cincinnati.Graph, err error) {
	arch := cs.CincinnatiParams.Arch
	if cs.Opts.Mode == mirror.DiskToMirror {
		return loadGraphDataFromDisk(cs.CincinnatiParams.GraphDataDir, arch, channel)
	}

	opts := []cincinnati.Option{
		cincinnati.WithChannel(channel),
		cincinnati.WithID(cs.Client.GetID()),
		cincinnati.WithURL(cs.Client.GetURL().String()),
		cincinnati.WithTransport(cs.Client.GetTransport()),
	}
	if len(arch) > 0 {
		opts = append(opts, cincinnati.WithArch(arch))
	}
	if len(version) > 0 {
		opts = append(opts, cincinnati.WithVersion(version))
	}

	// Download the update graph.
	data, err := cincinnati.DownloadGraphData(ctx, cs.Log, opts...)
	if err != nil {
		return graph, &Error{Reason: "GetGraphDataFailed", Message: err.Error(), cause: err}
	}

	graph, err = cincinnati.LoadGraphData(data)
	if err != nil {
		return graph, &Error{Reason: "ResponseInvalid", Message: err.Error(), cause: err}
	}

	if err := writeGraphDataToFile(data, arch, channel, cs.CincinnatiParams.GraphDataDir); err != nil {
		return graph, err
	}

	return graph, nil
}

func loadGraphDataFromDisk(graphDataDir, arch, channel string) (cincinnati.Graph, error) {
	var graph cincinnati.Graph
	filename := fmt.Sprintf("%s-%s.json", arch, channel)
	filepath := path.Join(graphDataDir, filename)
	if _, err := os.Stat(filepath); err != nil {
		return graph, &Error{Reason: "NoGraphData", Message: "No graph data found on disk"}
	}

	fileData, err := os.ReadFile(filepath)
	if err != nil {
		return graph, &Error{Reason: "ReadFileFailed", Message: err.Error(), cause: err}
	}

	if graph, err = cincinnati.LoadGraphData(fileData); err != nil {
		return graph, &Error{Reason: "GraphDataInvalid", Message: fmt.Sprintf("graph data %s: %v", filepath, err), cause: err}
	}

	return graph, nil
}

func writeGraphDataToFile(body []byte, arch, channel, graphDataDir string) error {
	filename := fmt.Sprintf("%s-%s.json", arch, channel)
	if err := os.WriteFile(path.Join(graphDataDir, filename), body, 0o644); err != nil { //nolint:gosec // no sensitive info
		return &Error{Reason: "FileWriteFailed", Message: err.Error(), cause: err}
	}

	return nil
}
