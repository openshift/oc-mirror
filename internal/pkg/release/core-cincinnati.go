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

	edgesByOrigin := upgradeEdges(graph, nil)

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

	// The next minor version channel can never reach a later major version,
	// so cross major version upgrades are calculated from the channel graphs.
	if source.Major < target.Major {
		return calculateCrossMajor(ctx, cs, sourceChannel, targetChannel, startVer, reqVer)
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

// calculateCrossMajor calculates the upgrades from startVer to reqVer when the target channel
// belongs to a later major version than the source channel.
//
// The channel that follows the source channel cannot be derived by incrementing the minor
// version across a major version boundary, so the channel graphs are used instead:
//   - When the target channel contains startVer, the shortest path is calculated in the
//     target channel.
//   - Otherwise, the target channel is entered from the latest of its releases of an earlier
//     minor version that leads to reqVer and that is reachable from startVer in the channel of
//     that release (<source prefix>-<major>.<minor>).
//
// When startVer and reqVer are generally available releases, the upgrades only go through
// generally available releases. When there is no upgrade path, an error naming the source and
// target channels is returned.
func calculateCrossMajor(ctx context.Context, cs CincinnatiSchema, sourceChannel, targetChannel string, startVer, reqVer semver.Version) (cincinnati.Update, cincinnati.Update, []cincinnati.Update, error) {
	source, target, prefix, err := getSemverFromChannels(sourceChannel, targetChannel)
	if err != nil {
		return cincinnati.Update{}, cincinnati.Update{}, nil, err
	}

	targetGraph, err := getChannelGraph(ctx, cs, targetChannel, startVer)
	if err != nil {
		return cincinnati.Update{}, cincinnati.Update{}, nil, err
	}
	requested, reqIdx, err := targetGraph.GetNodeByVersion(reqVer)
	if err != nil {
		return cincinnati.Update{}, cincinnati.Update{}, nil, noUpgradePathError(sourceChannel, targetChannel, startVer, reqVer,
			fmt.Sprintf("version %s is not in channel %q", reqVer, targetChannel))
	}
	include := upgradeFilter(startVer, reqVer)
	targetEdges := upgradeEdges(targetGraph, include)

	if current, startIdx, err := targetGraph.GetNodeByVersion(startVer); err == nil {
		cs.Log.Debug("Getting updates for version %s in channel %s", startVer.String(), targetChannel)
		path := shortestPath(targetEdges, startIdx, reqIdx)
		if len(path) == 0 {
			return cincinnati.Update{}, cincinnati.Update{}, nil, noUpgradePathError(sourceChannel, targetChannel, startVer, reqVer,
				fmt.Sprintf("channel %q has no path from %s to %s%s", targetChannel, startVer, reqVer, filterDescription(include)))
		}
		return current, requested, graphUpdates(targetGraph, path), nil
	}

	minVer := semver.Version{Major: startVer.Major, Minor: startVer.Minor}
	if minVer.LT(source) {
		minVer = source
	}
	candidates := entryCandidates(targetGraph, targetEdges, minVer, target, startVer, reqIdx)
	current, entryUpdates, entryIdx, err := enterTargetChannel(ctx, cs, prefix, targetGraph, candidates, include, startVer)
	if err != nil {
		return cincinnati.Update{}, cincinnati.Update{}, nil, err
	}
	if len(entryUpdates) == 0 {
		return cincinnati.Update{}, cincinnati.Update{}, nil, noUpgradePathError(sourceChannel, targetChannel, startVer, reqVer,
			fmt.Sprintf("channel %q contains neither version %s nor a release of an earlier minor version that is reachable from it and leads to %s%s",
				targetChannel, startVer, reqVer, filterDescription(include)))
	}
	cs.Log.Debug("Entering channel %s from version %s", targetChannel, targetGraph.Nodes[entryIdx].Version.String())
	path := shortestPath(targetEdges, entryIdx, reqIdx)
	return current, requested, append(entryUpdates, graphUpdates(targetGraph, path[1:])...), nil
}

// entryCandidates returns the indexes of the target graph releases from which the target
// channel can be entered, sorted by descending version: releases later than startVer, of a
// minor version from minVer up to (excluding) target, that lead to the release at reqIdx
// through edges. As edges only connect accepted releases (see upgradeEdges), so do candidates.
func entryCandidates(targetGraph cincinnati.Graph, targetEdges map[int][]int, minVer, target, startVer semver.Version, reqIdx int) []int {
	leadsToReq := reachableTo(targetEdges, reqIdx)
	var candidates []int
	for idx, node := range targetGraph.Nodes {
		minor := semver.Version{Major: node.Version.Major, Minor: node.Version.Minor}
		if idx == reqIdx || !leadsToReq[idx] || !node.Version.GT(startVer) || minor.LT(minVer) || !minor.LT(target) {
			continue
		}
		candidates = append(candidates, idx)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return targetGraph.Nodes[candidates[i]].Version.GT(targetGraph.Nodes[candidates[j]].Version)
	})
	return candidates
}

// enterTargetChannel tries the entry candidates of the target graph in order and returns the
// upgrades from startVer to the first candidate that is reachable from startVer in the channel
// of the candidate's minor version, along with the candidate index. The channel of each minor
// version is fetched at most once. No upgrades are returned when no candidate is reachable.
func enterTargetChannel(ctx context.Context, cs CincinnatiSchema, prefix string, targetGraph cincinnati.Graph, candidates []int, include func(cincinnati.Update) bool, startVer semver.Version) (cincinnati.Update, []cincinnati.Update, int, error) {
	graphs := map[string]cincinnati.Graph{}
	for _, idx := range candidates {
		entryVer := targetGraph.Nodes[idx].Version
		entryChannel := fmt.Sprintf("%s-%d.%d", prefix, entryVer.Major, entryVer.Minor)
		graph, ok := graphs[entryChannel]
		if !ok {
			var err error
			if graph, err = getChannelGraph(ctx, cs, entryChannel, startVer); err != nil {
				return cincinnati.Update{}, nil, -1, err
			}
			graphs[entryChannel] = graph
		}
		if path := versionPath(graph, include, startVer, entryVer); len(path) > 0 {
			cs.Log.Debug("Getting updates for version %s in channel %s", startVer.String(), entryChannel)
			return cincinnati.Update(graph.Nodes[path[0]]), graphUpdates(graph, path), idx, nil
		}
	}
	return cincinnati.Update{}, nil, -1, nil
}

// versionPath returns the shortest path from one version to another in the graph through the
// releases accepted by include, or nil when the graph does not contain both versions or has no
// path between them.
func versionPath(graph cincinnati.Graph, include func(cincinnati.Update) bool, from, to semver.Version) []int {
	_, fromIdx, err := graph.GetNodeByVersion(from)
	if err != nil {
		return nil
	}
	_, toIdx, err := graph.GetNodeByVersion(to)
	if err != nil {
		return nil
	}
	return shortestPath(upgradeEdges(graph, include), fromIdx, toIdx)
}

// upgradeFilter returns the releases that a cross major version upgrade may go through: only
// generally available releases when both startVer and reqVer are generally available, and all
// releases (nil) otherwise.
func upgradeFilter(startVer, reqVer semver.Version) func(cincinnati.Update) bool {
	if len(startVer.Pre) > 0 || len(reqVer.Pre) > 0 {
		return nil
	}
	return func(update cincinnati.Update) bool {
		return len(update.Version.Pre) == 0
	}
}

// filterDescription describes the restriction applied by upgradeFilter in error messages.
func filterDescription(include func(cincinnati.Update) bool) string {
	if include == nil {
		return ""
	}
	return " through generally available releases"
}

// noUpgradePathError returns the error for a missing upgrade path from startVer in the source
// channel to reqVer in the target channel.
func noUpgradePathError(sourceChannel, targetChannel string, startVer, reqVer semver.Version, detail string) error {
	return &Error{
		Reason:  "NoUpgradePath",
		Message: fmt.Sprintf("no upgrade path from version %s in channel %q to version %s in channel %q: %s", startVer, sourceChannel, reqVer, targetChannel, detail),
	}
}

// getChannelGraph fetches the update graph of the channel.
func getChannelGraph(ctx context.Context, cs CincinnatiSchema, channel string, version semver.Version) (cincinnati.Graph, error) {
	graph, err := getGraphData(ctx, cs, channel, version.String())
	if err != nil {
		return graph, &Error{
			Reason:  "APIRequestError",
			Message: fmt.Sprintf(ChannelInfo, channel, err),
			cause:   err,
		}
	}
	return graph, nil
}

// graphUpdates returns the updates of the graph nodes on the path.
func graphUpdates(graph cincinnati.Graph, path []int) []cincinnati.Update {
	updates := make([]cincinnati.Update, 0, len(path))
	for _, idx := range path {
		updates = append(updates, cincinnati.Update(graph.Nodes[idx]))
	}
	return updates
}

// upgradeEdges returns the destinations of the graph edges by origin node index, sorted by
// descending version to ensure deterministic results. When include is not nil, only edges
// between releases it accepts are returned.
func upgradeEdges(graph cincinnati.Graph, include func(cincinnati.Update) bool) map[int][]int {
	edgesByOrigin := make(map[int][]int, len(graph.Nodes))
	for _, edge := range graph.Edges {
		if include != nil && (!include(cincinnati.Update(graph.Nodes[edge[0]])) || !include(cincinnati.Update(graph.Nodes[edge[1]]))) {
			continue
		}
		edgesByOrigin[edge[0]] = append(edgesByOrigin[edge[0]], edge[1])
	}

	// Sort destination by semver to ensure deterministic result
	for origin, destinations := range edgesByOrigin {
		sort.Slice(destinations, func(i, j int) bool {
			return graph.Nodes[destinations[i]].Version.GT(graph.Nodes[destinations[j]].Version)
		})
		edgesByOrigin[origin] = destinations
	}
	return edgesByOrigin
}

// shortestPath returns the node indexes of the shortest path from start to end through the
// edges, or an empty slice when end cannot be reached from start.
func shortestPath(g map[int][]int, start, end int) []int {
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

// reachableTo returns the indexes of the nodes from which end can be reached through the
// edges, including end itself.
func reachableTo(edgesByOrigin map[int][]int, end int) map[int]bool {
	origins := make(map[int][]int, len(edgesByOrigin))
	for origin, destinations := range edgesByOrigin {
		for _, destination := range destinations {
			origins[destination] = append(origins[destination], origin)
		}
	}

	reachable := map[int]bool{end: true}
	queue := []int{end}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		for _, origin := range origins[node] {
			if !reachable[origin] {
				reachable[origin] = true
				queue = append(queue, origin)
			}
		}
	}
	return reachable
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
		return source, target, prefix, fmt.Errorf("%w", err)
	}
	target, err = semver.Parse(fmt.Sprintf("%s.0", targetChannel[targetIdx+1:]))
	if err != nil {
		return source, target, prefix, fmt.Errorf("%w", err)
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
