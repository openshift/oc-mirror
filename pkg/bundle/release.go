package bundle

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	semver "github.com/blang/semver/v4"
	"github.com/google/uuid"
	"github.com/openshift/oc/pkg/cli/admin/release"
	"github.com/sirupsen/logrus"

	"github.com/RedHatGov/bundle/pkg/cli"
	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
	"github.com/RedHatGov/bundle/pkg/image"
)

// This file is for managing OCP release related tasks

// import(
//   "github.com/openshift/cluster-version-operator/pkg/cincinnati"
// )

// ReleaseOptions configures either a Full or Diff mirror operation
// on a particular release image.
type ReleaseOptions struct {
	cli.RootOptions
}

// NewReleaseOptions defaults ReleaseOptions.
func NewReleaseOptions(ro cli.RootOptions) *ReleaseOptions {
	return &ReleaseOptions{RootOptions: ro}
}

// Define interface and var for http client to support testing
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

var (
	HClient HTTPClient
)

const (
	UpdateUrl string = "https://api.openshift.com/api/upgrades_info/v1/graph"
	// Does not currently handle arch selection
	arch = "x86_64"
)

func init() {
	HClient = &http.Client{}
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

func newClient() (Client, *url.URL, error) {
	upstream, err := url.Parse(UpdateUrl)
	if err != nil {
		return Client{}, nil, err
	}

	// This needs to handle user input at some point.
	var proxy *url.URL

	tls, err := getTLSConfig()
	if err != nil {
		return Client{}, nil, err
	}
	return NewClient(uuid.New(), proxy, tls), upstream, nil
}

// Next calculate the upgrade path from the current version to the channel's latest
func calculateUpgradePath(ch v1alpha1.ReleaseChannel, v semver.Version) (Update, []Update, error) {

	client, upstream, err := newClient()
	if err != nil {
		return Update{}, nil, err
	}

	ctx := context.Background()

	channel := ch.Name

	upgrade, upgrades, err := client.GetUpdates(ctx, upstream, arch, channel, v)
	if err != nil {
		return Update{}, nil, err
	}

	return upgrade, upgrades, nil
	//	// get latest channel version dowloaded

	//	// output a channel (struct) with the upgrade versions needed
	//
}

//func downloadRelease(c channel) error {
//	// Download the referneced versions by channel
//}
func GetLatestVersion(ch v1alpha1.ReleaseChannel) (Update, error) {

	client, upstream, err := newClient()
	if err != nil {
		return Update{}, err
	}

	ctx := context.Background()

	channel := ch.Name

	latest, err := client.GetChannelLatest(ctx, upstream, arch, channel)
	if err != nil {
		return Update{}, err
	}
	upgrade, _, err := client.GetUpdates(ctx, upstream, arch, channel, latest)
	if err != nil {
		return Update{}, err
	}

	return upgrade, err
	//	// get latest channel version dowloaded

	//	// output a channel (struct) with the upgrade versions needed
	//
}

func (c Client) GetChannelLatest(ctx context.Context, uri *url.URL, arch string, channel string) (semver.Version, error) {
	var latest Update
	transport := http.Transport{}
	// Prepare parametrized cincinnati query.
	queryParams := uri.Query()
	//queryParams.Add("arch", arch)
	queryParams.Add("channel", channel)
	queryParams.Add("id", c.id.String())
	uri.RawQuery = queryParams.Encode()

	// Download the update graph.
	req, err := http.NewRequest("GET", uri.String(), nil)
	if err != nil {
		return latest.Version, &Error{Reason: "InvalidRequest", Message: err.Error(), cause: err}
	}
	req.Header.Add("Accept", GraphMediaType)
	if c.tlsConfig != nil {
		transport.TLSClientConfig = c.tlsConfig
	}

	if c.proxyURL != nil {
		transport.Proxy = http.ProxyURL(c.proxyURL)
	}

	//HClient = &http.Client{Transport: &transport}
	hc, ok := HClient.(*http.Client)
	if ok {
		hc.Transport = &transport
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, getUpdatesTimeout)
	defer cancel()
	resp, err := HClient.Do(req.WithContext(timeoutCtx))
	if err != nil {
		return latest.Version, &Error{Reason: "RemoteFailed", Message: err.Error(), cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return latest.Version, &Error{Reason: "ResponseFailed", Message: fmt.Sprintf("unexpected HTTP status: %s", resp.Status)}
	}

	// Parse the graph.
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return latest.Version, &Error{Reason: "ResponseFailed", Message: err.Error(), cause: err}
	}

	var graph graph
	if err = json.Unmarshal(body, &graph); err != nil {
		return latest.Version, &Error{Reason: "ResponseInvalid", Message: err.Error(), cause: err}
	}

	// Find the current version within the graph.
	Vers := []semver.Version{}
	for _, node := range graph.Nodes {

		Vers = append(Vers, node.Version)
	}

	semver.Sort(Vers)
	new := Vers[len(Vers)-1]

	return new, err
}

func (o *ReleaseOptions) downloadMirror(secret []byte, toDir, from string) (image.Associations, error) {
	opts := release.NewMirrorOptions(o.IOStreams)
	opts.From = from
	opts.ToDir = toDir

	// If the pullSecret is not empty create a cached context
	// else let `oc mirror` use the default docker config location
	if len(secret) != 0 {
		ctx, err := config.CreateContext(secret, o.SkipVerification, o.SkipTLS)
		if err != nil {
			return image.Associations{}, err
		}
		opts.SecurityOptions.CachedContext = ctx
	}

	opts.SecurityOptions.Insecure = o.SkipTLS
	opts.SecurityOptions.SkipVerification = o.SkipVerification
	opts.DryRun = o.DryRun

	if err := opts.Run(); err != nil {
		return image.Associations{}, err
	}

	// Retrive the mapping information for release
	mapping, images, err := o.getMapping(*opts)

	if err != nil {
		return image.Associations{}, fmt.Errorf("error could retrieve mapping information: %v", err)
	}

	assocs, err := image.AssociateImageLayers(toDir, mapping, images)
	if err != nil {
		return nil, err
	}
	for k, assoc := range assocs {
		assoc.Type = image.TypeOCPRelease
		assocs[k] = assoc
	}

	return assocs, nil
}

func (o *ReleaseOptions) GetReleasesInitial(cfg v1alpha1.ImageSetConfiguration) (image.Associations, error) {

	allAssocs := image.Associations{}
	pullSecret := cfg.Mirror.OCP.PullSecret
	srcDir := filepath.Join(o.Dir, config.SourceDir)

	// For each channel in the config file
	for _, ch := range cfg.Mirror.OCP.Channels {

		if len(ch.Versions) == 0 {
			// If no version was specified from the channel, then get the latest release
			latest, err := GetLatestVersion(ch)
			if err != nil {
				return image.Associations{}, err
			}
			logrus.Infof("Image to download: %v", latest.Image)
			// Download the release
			assocs, err := o.downloadMirror([]byte(pullSecret), srcDir, latest.Image)
			if err != nil {
				return image.Associations{}, err
			}
			allAssocs.Merge(assocs)
			logrus.Infof("Channel Latest version %v", latest.Version)
		}

		// Check for specific version declarations for each specific version
		for _, v := range ch.Versions {

			// Convert the string to a semver
			ver, err := semver.Parse(v)

			if err != nil {
				return image.Associations{}, err
			}

			// This dumps the available upgrades from the last downloaded version
			requested, _, err := calculateUpgradePath(ch, ver)
			if err != nil {
				return image.Associations{}, fmt.Errorf("failed to get upgrade graph: %v", err)
			}

			logrus.Infof("requested: %v", requested.Version)
			assocs, err := o.downloadMirror([]byte(pullSecret), srcDir, requested.Image)
			if err != nil {
				return image.Associations{}, err
			}
			allAssocs.Merge(assocs)
			logrus.Infof("Channel Latest version %v", requested.Version)

			/* Select the requested version from the available versions
			for _, d := range neededVersions {
				logrus.Infof("Available Release Version: %v \n Requested Version: %o", d.Version, rs)
				if d.Version.Equals(rs) {
					logrus.Infof("Image to download: %v", d.Image)
					err := downloadMirror(d.Image)
					if err != nil {
						logrus.Errorln(err)
					}
					logrus.Infof("Image to download: %v", d.Image)
					break
				}
			} */

			// download the selected version

			logrus.Infof("Current Object: %v", v)
			//logrus.Infof("Next-Versions: %v", neededVersions.)
			//nv = append(nv, neededVersions)
		}
	}

	// Download each referenced version from
	//downloadRelease(nv)

	return allAssocs, nil
}

func (o *ReleaseOptions) GetReleasesDiff(_ v1alpha1.PastMirror, cfg v1alpha1.ImageSetConfiguration) (image.Associations, error) {

	allAssocs := image.Associations{}
	pullSecret := cfg.Mirror.OCP.PullSecret
	srcDir := filepath.Join(o.Dir, config.SourceDir)

	for _, ch := range cfg.Mirror.OCP.Channels {
		// Check for specific version declarations for each specific version
		for _, v := range ch.Versions {

			// Convert the string to a semver
			ver, err := semver.Parse(v)

			if err != nil {
				return image.Associations{}, err
			}

			// This dumps the available upgrades from the last downloaded version
			requested, _, err := calculateUpgradePath(ch, ver)
			if err != nil {
				return image.Associations{}, fmt.Errorf("failed to get upgrade graph: %v", err)
			}

			logrus.Infof("requested: %v", requested.Version)
			assocs, err := o.downloadMirror([]byte(pullSecret), srcDir, requested.Image)
			if err != nil {
				return image.Associations{}, err
			}
			allAssocs.Merge(assocs)

			logrus.Infof("Channel Latest version %v", requested.Version)

			/* Select the requested version from the available versions
			for _, d := range neededVersions {
				logrus.Infof("Available Release Version: %v \n Requested Version: %o", d.Version, rs)
				if d.Version.Equals(rs) {
					logrus.Infof("Image to download: %v", d.Image)
					err := downloadMirror(d.Image)
					if err != nil {
						logrus.Errorln(err)
					}
					logrus.Infof("Image to download: %v", d.Image)
					break
				}
			} */

			// download the selected version

			logrus.Infof("Current Object: %v", v)
			//logrus.Infof("Next-Versions: %v", neededVersions.)
			//nv = append(nv, neededVersions
		}
	}

	// Download each referenced version from
	//downloadRelease(nv)

	return allAssocs, nil
}

// getMapping will run release mirror with ToMirror set to true to get mapping information
func (o *ReleaseOptions) getMapping(opts release.MirrorOptions) (mappings map[string]string, images []string, err error) {

	mappingPath := filepath.Join(o.Dir, "release-mapping.txt")
	file, err := os.Create(mappingPath)

	defer os.Remove(mappingPath)

	if err != nil {
		return mappings, images, err
	}

	// Run release mirror with ToMirror set to retrieve mapping information
	// store in buffer for manipulation before outputting to mapping.txt
	var buffer bytes.Buffer
	opts.IOStreams.Out = &buffer

	opts.ToMirror = true

	if err := opts.Run(); err != nil {
		return mappings, images, err
	}

	scanner := bufio.NewScanner(&buffer)

	// Scan mapping output and write to file
	for scanner.Scan() {
		text := scanner.Text()
		split := strings.Split(text, " ")

		// Proccess name and add arch to dir name
		// TODO: architecture handling
		var names []string
		name := opts.TargetFn(split[1]).Exact()
		nameSplit := strings.Split(name, "-")
		names = []string{nameSplit[1], arch}
		names = append(names, nameSplit[2:]...)
		name = strings.Join(names, "-")

		if _, err := file.WriteString(split[0] + "=" + name + "\n"); err != nil {
			return mappings, images, err
		}
		images = append(images, split[0])
	}

	mappings, err = image.ReadImageMapping(mappingPath)

	if err != nil {
		return mappings, images, err
	}

	return mappings, images, nil
}
