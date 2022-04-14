package cincinnati

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/google/uuid"
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
)

// Client is a Cincinnati client which can be used to fetch update graphs from
// an upstream Cincinnati stack.
type Client struct {
	id        uuid.UUID
	transport *http.Transport
}

// NewClient creates a new Cincinnati client with the given client identifier.
func NewClient(u string, id uuid.UUID) (Client, *url.URL, error) {
	upstream, err := url.Parse(u)
	if err != nil {
		return Client{}, nil, err
	}

	tls, err := getTLSConfig()
	if err != nil {
		return Client{}, nil, err
	}

	transport := &http.Transport{
		TLSClientConfig: tls,
		Proxy:           http.ProxyFromEnvironment,
	}
	return Client{id: id, transport: transport}, upstream, nil
}

func getTLSConfig() (*tls.Config, error) {
	certPool, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	config := &tls.Config{
		RootCAs: certPool,
	}
	return config, nil
}

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
func (c Client) GetUpdates(ctx context.Context, uri *url.URL, arch string, channel string, version semver.Version, reqVer semver.Version) (Update, Update, []Update, error) {
	var current Update
	var requested Update
	// Prepare parametrized cincinnati query.
	queryParams := uri.Query()
	if channel != "okd" {
		queryParams.Add("arch", arch)
		queryParams.Add("channel", channel)
		queryParams.Add("id", c.id.String())
		queryParams.Add("version", version.String())
	}
	uri.RawQuery = queryParams.Encode()

	graph, err := c.getGraphData(ctx, uri)
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

	// Find the children of the current version.
	var nextIdxs []int
	for _, edge := range graph.Edges {
		if edge.Origin == currentIdx && edge.Destination == destinationIdx {
			nextIdxs = append(nextIdxs, edge.Destination)
		}
	}

	var updates []Update
	for _, i := range nextIdxs {
		updates = append(updates, Update(graph.Nodes[i]))
	}

	return current, requested, updates, nil
}

// CalculateUpgrades fetches and calculates all the update payloads from the specified
// upstream Cincinnati stack given the current and target version and channel
func (c Client) CalculateUpgrades(ctx context.Context, uri *url.URL, arch, sourceChannel, targetChannel string, version, reqVer semver.Version) (current, requested Update, upgrades []Update, err error) {
	// If the we are staying in the same channel
	if sourceChannel == targetChannel {
		return c.GetUpdates(ctx, uri, arch, targetChannel, version, reqVer)
	}

	// Get semver representation of source and target channel versions
	source, target, prefix, err := getSemverFromChannels(sourceChannel, targetChannel)
	if err != nil {
		return current, requested, upgrades, err
	}

	if source.EQ(target) {
		isBlocked, err := c.handleBlockedEdges(ctx, uri, arch, targetChannel, version)
		if err != nil {
			return Update{}, Update{}, nil, err
		}
		if isBlocked {
			// If blocked path is found, just return the requested version
			logrus.Warnf("No upgrade path for %s in target channel %s", version.String(), targetChannel)
			return c.GetUpdates(ctx, uri, arch, targetChannel, reqVer, reqVer)
		}
		return c.GetUpdates(ctx, uri, arch, targetChannel, version, reqVer)
	}

	// Get latest version from sourceChannel
	latest, err := c.GetChannelLatest(ctx, uri, arch, sourceChannel)
	if err != nil {
		return current, requested, upgrades, fmt.Errorf("cannot get latest: %v", err)
	}
	current, _, upgrades, err = c.GetUpdates(ctx, uri, arch, sourceChannel, version, latest)
	if err != nil {
		return current, requested, upgrades, fmt.Errorf("cannot get current: %v", err)
	}

	for {
		// Bump the minor version on the channel
		source.Minor++
		currChannel := fmt.Sprintf("%s-%v.%v", prefix, source.Major, source.Minor)
		logrus.Debugf("Processing channel %s", currChannel)

		// Get versions in channel
		versions, err := c.GetVersions(ctx, uri, currChannel)
		if err != nil {
			return current, requested, upgrades, err
		}
		foundVersions := make(map[string]struct{})
		for _, v := range versions {
			foundVersions[v.String()] = struct{}{}
		}

		var newLatest semver.Version
		if source.EQ(target) {
			// If this is the target channel get
			// requested version
			newLatest = reqVer
			currChannel = targetChannel
		} else {
			// Get the latest from current channel
			newLatest, err = c.GetChannelLatest(ctx, uri, arch, currChannel)
			if err != nil {
				return current, requested, upgrades, err
			}
		}

		// If the previous latest version exists in this channel then get updates
		logrus.Debugf("Getting updates for latest %s in channel %s", latest.String(), currChannel)
		isBlocked, err := c.handleBlockedEdges(ctx, uri, arch, currChannel, latest)
		if err != nil {
			return current, requested, upgrades, err
		}
		if isBlocked {
			// return nil instead of upgrades
			// so we don't process an incomplete upgrade path
			_, requested, _, err = c.GetUpdates(ctx, uri, arch, targetChannel, reqVer, reqVer)
			logrus.Warnf("No upgrade path for %s in target channel %s", latest.String(), targetChannel)
			return current, requested, nil, err
		}

		_, req, currUpgrades, err := c.GetUpdates(ctx, uri, arch, currChannel, latest, newLatest)
		if err != nil {
			return current, requested, upgrades, err
		}
		upgrades = append(upgrades, currUpgrades...)
		requested = req

		latest = newLatest
		if source.EQ(target) {
			break
		}
	}

	return current, requested, upgrades, err
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

// handleBlockedEdges will check for the the starting version in the current channel
// if it does not exists the version is blocked.
func (c Client) handleBlockedEdges(ctx context.Context, uri *url.URL, arch, targetChannel string, startVer semver.Version) (bool, error) {
	chanVersions, err := c.GetVersions(ctx, uri, targetChannel)
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

// GetChannelLatest fetches the latest version from the specified
// upstream Cincinnati stack given architecture and channel
func (c Client) GetChannelLatest(ctx context.Context, uri *url.URL, arch string, channel string) (semver.Version, error) {
	// Prepare parametrized cincinnati query.
	queryParams := uri.Query()
	if channel != "okd" {
		queryParams.Add("arch", arch)
		queryParams.Add("channel", channel)
		queryParams.Add("id", c.id.String())
	}
	uri.RawQuery = queryParams.Encode()

	graph, err := c.getGraphData(ctx, uri)
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

	return Vers[len(Vers)-1], nil
}

// GetChannels fetches the channels containing update payloads from the specified
// upstream Cincinnati stack
func (c Client) GetChannels(ctx context.Context, uri *url.URL, channel string) (map[string]struct{}, error) {
	// Prepare parametrized cincinnati query.
	queryParams := uri.Query()
	if channel != "okd" {
		queryParams.Add("channel", channel)
		queryParams.Add("id", c.id.String())
	}
	uri.RawQuery = queryParams.Encode()

	graph, err := c.getGraphData(ctx, uri)
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
func (c Client) GetVersions(ctx context.Context, uri *url.URL, channel string) ([]semver.Version, error) {
	// Prepare parametrized cincinnati query.
	queryParams := uri.Query()
	if channel != "okd" {
		queryParams.Add("channel", channel)
		queryParams.Add("id", c.id.String())
	}
	uri.RawQuery = queryParams.Encode()

	graph, err := c.getGraphData(ctx, uri)
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
func (c Client) getGraphData(ctx context.Context, uri *url.URL) (graph graph, err error) {
	// Download the update graph.
	req, err := http.NewRequest("GET", uri.String(), nil)
	if err != nil {
		return graph, &Error{Reason: "InvalidRequest", Message: err.Error(), cause: err}
	}
	req.Header.Add("Accept", GraphMediaType)
	if c.transport != nil && c.transport.TLSClientConfig != nil {
		if c.transport.TLSClientConfig.ClientCAs == nil {
			klog.V(5).Infof("Using a root CA pool with 0 root CA subjects to request updates from %s", uri)
		} else {
			klog.V(5).Infof("Using a root CA pool with %n root CA subjects to request updates from %s", len(c.transport.TLSClientConfig.RootCAs.Subjects()), uri)
		}
	}

	if c.transport != nil && c.transport.Proxy != nil {
		proxy, err := c.transport.Proxy(req)
		if err == nil && proxy != nil {
			klog.V(5).Infof("Using proxy %s to request updates from %s", proxy.Host, uri)
		}
	}

	client := http.Client{}
	if c.transport != nil {
		client.Transport = c.transport
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
