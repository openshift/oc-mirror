package release

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/blang/semver/v4"

	"github.com/openshift/oc-mirror/v2/internal/pkg/cincinnati"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

const ChannelInfo = "channel %q: %v"

const (
	reasonAPIRequestError = "APIRequestError"
	reasonVersionNotFound = "VersionNotFound"
	reasonFileWriteFailed = "FileWriteFailed"
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
			Reason:  reasonAPIRequestError,
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
			Reason:  reasonVersionNotFound,
			Message: fmt.Sprintf("current version %s not found in the %q channel", version, channel),
		}
	}

	requested, destinationIdx, err := graph.GetNodeByVersion(reqVer)
	if errors.Is(err, cincinnati.ErrVersionNotFound) {
		return current, requested, nil, &Error{
			Reason:  reasonVersionNotFound,
			Message: fmt.Sprintf("requested version %s not found in the %q channel", reqVer, channel),
		}
	}

	edgesByOrigin := upgradeEdges(graph, false)

	nextIdxs := shortestPath(edgesByOrigin, currentIdx, destinationIdx)
	return current, requested, graphUpdates(graph, nextIdxs), nil
}

// CalculateUpgrades fetches and calculates all the update payloads from the specified
// upstream Cincinnati stack given the current and target version and channel. When the target
// graph has no path to a later major version, the search also checks the source graph.
func CalculateUpgrades(ctx context.Context, cs CincinnatiSchema, sourceChannel, targetChannel string, startVer, reqVer semver.Version) (cincinnati.Update, cincinnati.Update, []cincinnati.Update, error) {
	return calculateUpgrades(ctx, cs, []string{sourceChannel}, []string{targetChannel}, startVer, reqVer)
}

// calculateUpgrades is CalculateUpgrades with the source and target channel candidates: the selected
// channels first, then the other configured channels with the same start or target version. Only
// the upgrades to a later major version use the other candidates.
func calculateUpgrades(ctx context.Context, cs CincinnatiSchema, sources, targets []string, startVer, reqVer semver.Version) (cincinnati.Update, cincinnati.Update, []cincinnati.Update, error) { //nolint:cyclop // FIXME: needs further refactoring.
	sourceChannel, targetChannel := sources[0], targets[0]
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

	// The usual search only increments the minor version, so it would
	// look for 4.23 after 4.22 even when the target is 5.0.
	// For a later major version, use calculateCrossMajor to find where
	// the channel graphs actually allow the jump.
	if source.Major < target.Major {
		return calculateCrossMajor(ctx, cs, sources, targets, startVer, reqVer)
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

// calculateCrossMajor looks for a path in the target graph, then in both graphs together. When
// neither has a path, it tries the other source and target candidates before warning.
func calculateCrossMajor(ctx context.Context, cs CincinnatiSchema, sources, targets []string, startVer, reqVer semver.Version) (cincinnati.Update, cincinnati.Update, []cincinnati.Update, error) {
	sourceChannel, targetChannel := sources[0], targets[0]
	graphs := &graphCache{cs: cs, startVer: startVer, graphs: map[string]cincinnati.Graph{}}

	// Try the target first: it may already have the whole path.
	targetGraph, err := graphs.load(ctx, targetChannel)
	if err != nil {
		return cincinnati.Update{}, cincinnati.Update{}, nil, err
	}
	if !hasVersion(targetGraph, reqVer) {
		return cincinnati.Update{}, cincinnati.Update{}, nil, &Error{
			Reason:  reasonVersionNotFound,
			Message: fmt.Sprintf("requested version %s not found in the %q channel", reqVer, targetChannel),
		}
	}

	// Don't route between two final releases through a prerelease.
	gaOnly := len(startVer.Pre) == 0 && len(reqVer.Pre) == 0
	current, requested, updates, _ := upgradePath(targetGraph, startVer, reqVer, gaOnly)
	if len(updates) > 0 {
		return current, requested, updates, nil
	}

	// The source may have missing links. Merge target first to keep its image references.
	sourceGraph, err := graphs.load(ctx, sourceChannel)
	if err != nil {
		return cincinnati.Update{}, cincinnati.Update{}, nil, err
	}
	combined := cincinnati.MergeGraphs(targetGraph, sourceGraph)
	current, requested, updates, startFound := upgradePath(combined, startVer, reqVer, gaOnly)
	if len(updates) > 0 {
		return current, requested, updates, nil
	}

	// Other configured channels may have the same start or target version and a path.
	known, tiedRequested, updates, startFound, err := graphs.tiedPath(ctx, tiedSearches(sources, targets), reqVer, gaOnly, current, startFound)
	if err != nil {
		return cincinnati.Update{}, cincinnati.Update{}, nil, err
	}
	if len(updates) > 0 {
		return known, tiedRequested, updates, nil
	}

	// Keep the known releases without claiming the cluster can upgrade.
	cs.Log.Warn("NoUpgradePath: no qualifying unconditional upgrade path from version %s in channel %q to version %s in channel %q for the %s architecture "+
		"(conditional update edges are not followed): %s; only the known start and target releases are mirrored for this upgrade",
		startVer, sourceChannel, reqVer, targetChannel, cs.CincinnatiParams.Arch, noPathReason(sources, targets, startVer, startFound, gaOnly))
	return known, requested, nil, nil
}

// graphCache loads each channel graph once per upgrade calculation.
type graphCache struct {
	cs       CincinnatiSchema
	startVer semver.Version
	graphs   map[string]cincinnati.Graph
}

// load returns the graph of the channel, loading it the first time.
func (c *graphCache) load(ctx context.Context, channel string) (cincinnati.Graph, error) {
	if graph, ok := c.graphs[channel]; ok {
		return graph, nil
	}
	graph, err := loadGraph(ctx, c.cs, channel, c.startVer)
	if err != nil {
		return graph, err
	}
	c.graphs[channel] = graph
	return graph, nil
}

// loadTied returns the graph of a source or target candidate. A candidate other than the selected
// pair reuses the graph saved by its per-channel collection, so that disk to mirror replays the
// graph that this run used; without a saved graph, it is loaded like the selected pair.
func (c *graphCache) loadTied(ctx context.Context, channel string) (cincinnati.Graph, error) {
	if graph, ok := c.graphs[channel]; ok {
		return graph, nil
	}
	dir, arch := c.cs.CincinnatiParams.GraphDataDir, c.cs.CincinnatiParams.Arch
	if _, err := os.Stat(path.Join(dir, fmt.Sprintf("%s-%s.json", arch, channel))); err != nil {
		return c.load(ctx, channel)
	}
	graph, err := loadGraphDataFromDisk(dir, arch, channel)
	if err != nil {
		return graph, &Error{Reason: reasonAPIRequestError, Message: fmt.Sprintf(ChannelInfo, channel, err), cause: err}
	}
	c.graphs[channel] = graph
	return graph, nil
}

// tiedSearch is a search in a target graph, alone or merged with a source graph.
type tiedSearch struct {
	target, source string
}

// tiedSearches returns the searches of the source and target candidates other than the selected
// pair, in order: for each target, its graph alone, then merged with each source graph.
func tiedSearches(sources, targets []string) []tiedSearch {
	var searches []tiedSearch
	for i, target := range targets {
		if i > 0 {
			searches = append(searches, tiedSearch{target: target})
		}
		for j, source := range sources {
			if i > 0 || j > 0 {
				searches = append(searches, tiedSearch{target: target, source: source})
			}
		}
	}
	return searches
}

// tiedPath runs the searches in order and returns the first path. Without a path, it returns the
// known start release, from the selected pair or a searched graph.
func (c *graphCache) tiedPath(ctx context.Context, searches []tiedSearch, reqVer semver.Version, gaOnly bool, known cincinnati.Update, startFound bool) (cincinnati.Update, cincinnati.Update, []cincinnati.Update, bool, error) {
	for _, search := range searches {
		// Cached and saved graphs do not check the context.
		if err := ctx.Err(); err != nil {
			return cincinnati.Update{}, cincinnati.Update{}, nil, false, fmt.Errorf("searching the tied channel graphs: %w", err)
		}
		graph, ok, err := c.searchGraph(ctx, search, reqVer)
		if err != nil {
			return cincinnati.Update{}, cincinnati.Update{}, nil, false, err
		}
		if !ok {
			continue
		}
		current, requested, updates, found := upgradePath(graph, c.startVer, reqVer, gaOnly)
		if len(updates) > 0 {
			return current, requested, updates, true, nil
		}
		if found && !startFound {
			known, startFound = current, true
		}
	}
	return known, cincinnati.Update{}, nil, startFound, nil
}

// searchGraph returns the graph of the search, or false when the target graph lacks the requested
// version or the source graph lacks the start version: a candidate stands only for its own version.
func (c *graphCache) searchGraph(ctx context.Context, search tiedSearch, reqVer semver.Version) (cincinnati.Graph, bool, error) {
	target, err := c.loadTied(ctx, search.target)
	if err != nil || !hasVersion(target, reqVer) {
		return cincinnati.Graph{}, false, err
	}
	if search.source == "" {
		return target, true, nil
	}
	source, err := c.loadTied(ctx, search.source)
	if err != nil || !hasVersion(source, c.startVer) {
		return cincinnati.Graph{}, false, err
	}
	return cincinnati.MergeGraphs(target, source), true, nil
}

// hasVersion returns whether the graph contains the version.
func hasVersion(graph cincinnati.Graph, version semver.Version) bool {
	_, _, err := graph.GetNodeByVersion(version)
	return err == nil
}

// noPathReason explains why the graphs of the source and target candidates have no path.
func noPathReason(sources, targets []string, startVer semver.Version, startFound, gaOnly bool) string {
	graphs, either := "the two graphs", "either graph"
	if len(sources)+len(targets) > 2 {
		graphs = "the graphs of the channels " + quotedChannels(sources, targets)
		either = graphs
	}
	if !startFound {
		return fmt.Sprintf("start version %s is not in %s", startVer, either)
	}
	reason := graphs + " publish no unconditional update path"
	if gaOnly {
		reason += " through generally available releases"
	}
	return reason
}

// quotedChannels returns the distinct names of the source and target candidates, sorted and quoted.
func quotedChannels(sources, targets []string) string {
	seen := make(map[string]bool, len(sources)+len(targets))
	names := make([]string, 0, len(sources)+len(targets))
	for _, name := range slices.Concat(sources, targets) {
		if !seen[name] {
			seen[name] = true
			names = append(names, strconv.Quote(name))
		}
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// loadGraph names the channel in errors and distinguishes failed saves from failed reads.
func loadGraph(ctx context.Context, cs CincinnatiSchema, channel string, startVer semver.Version) (cincinnati.Graph, error) {
	graph, err := getGraphData(ctx, cs, channel, startVer.String())
	if err == nil {
		return graph, nil
	}
	var storeErr *Error
	if errors.As(err, &storeErr) && storeErr.Reason == reasonFileWriteFailed {
		return graph, &Error{Reason: reasonFileWriteFailed, Message: fmt.Sprintf(ChannelInfo, channel, storeErr.Message), cause: err}
	}
	return graph, &Error{Reason: reasonAPIRequestError, Message: fmt.Sprintf(ChannelInfo, channel, err), cause: err}
}

// upgradePath searches a graph already known to contain the target release.
func upgradePath(graph cincinnati.Graph, startVer, reqVer semver.Version, gaOnly bool) (cincinnati.Update, cincinnati.Update, []cincinnati.Update, bool) {
	requested, reqIdx, _ := graph.GetNodeByVersion(reqVer)
	current, startIdx, err := graph.GetNodeByVersion(startVer)
	if err != nil {
		return current, requested, nil, false
	}
	updates := graphUpdates(graph, shortestPath(upgradeEdges(graph, gaOnly), startIdx, reqIdx))
	return current, requested, updates, true
}

// channelPrefix returns the prefix of the channel name, before its version.
func channelPrefix(channel string) string {
	return channel[:max(strings.LastIndex(channel, "-"), 0)]
}

// channelVersion returns the major and minor version of the channel name.
func channelVersion(channel string) (semver.Version, error) {
	idx := strings.LastIndex(channel, "-")
	if idx == -1 {
		return semver.Version{}, fmt.Errorf("invalid channel name %s", channel)
	}
	version, err := semver.Parse(channel[idx+1:] + ".0")
	if err != nil {
		return version, fmt.Errorf("invalid channel name %s: %w", channel, err)
	}
	return version, nil
}

// graphUpdates returns the updates of the graph nodes on the path.
func graphUpdates(graph cincinnati.Graph, path []int) []cincinnati.Update {
	if len(path) == 0 {
		return nil
	}
	updates := make([]cincinnati.Update, 0, len(path))
	for _, idx := range path {
		updates = append(updates, cincinnati.Update(graph.Nodes[idx]))
	}
	return updates
}

// upgradeEdges groups edges by their starting node and sorts destinations newest first.
// When gaOnly is true, it skips edges to or from prereleases.
func upgradeEdges(graph cincinnati.Graph, gaOnly bool) map[int][]int {
	edgesByOrigin := make(map[int][]int, len(graph.Nodes))
	for _, edge := range graph.Edges {
		if gaOnly && (len(graph.Nodes[edge[0]].Version.Pre) > 0 || len(graph.Nodes[edge[1]].Version.Pre) > 0) {
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
	if source, err = channelVersion(sourceChannel); err != nil {
		return source, target, prefix, err
	}
	if target, err = channelVersion(targetChannel); err != nil {
		return source, target, prefix, err
	}
	return source, target, channelPrefix(sourceChannel), nil
}

// GetChannelMinOrMax fetches the minimum or maximum version from the specified
// upstream Cincinnati stack given architecture and channel.
func GetChannelMinOrMax(ctx context.Context, cs CincinnatiSchema, channel string, min bool) (semver.Version, error) {
	graph, err := getGraphData(ctx, cs, channel, "")
	if err != nil {
		return semver.Version{}, &Error{
			Reason:  reasonAPIRequestError,
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
			Reason:  reasonAPIRequestError,
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
			Reason:  reasonAPIRequestError,
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
		return &Error{Reason: reasonFileWriteFailed, Message: err.Error(), cause: err}
	}

	return nil
}
