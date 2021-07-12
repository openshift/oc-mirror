package bundle

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	semver "github.com/blang/semver/v4"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// import(
//   "github.com/openshift/cluster-version-operator/pkg/cincinnati"
// )

// This file is for managing OCP release related tasks

func getTLSConfig() (*tls.Config, error) {
	certPool := x509.NewCertPool()

	if ok := certPool.AppendCertsFromPEM([]byte("/etc/ssl/cert.pem")); !ok {
		return nil, fmt.Errorf("unable to add ca-bundle.crt certificates")
	}

	config := &tls.Config{
		RootCAs: certPool,
	}

	return config, nil
}

// Next calculate the upgrade path from the current version to the channel's latest
func calculateUpgradePath(b channel, v *semver.Version) (Update, []Update, error) {

	upstream, err := url.Parse(UpdateUrl)
	if err != nil {
		logrus.Error(err)
	}

	// This needs to handle user input at some point.
	var proxy *url.URL

	var tls *tls.Config
	/*tls, err := getTLSConfig()
	if err != nil {
		logrus.Error(err)
	}*/
	client := NewClient(uuid.New(), proxy, tls)

	ctx := context.Background()

	// Does not currently handle arch selection
	arch := "x86_64"

	channel := b.Name

	upgrade, upgrades, err := client.GetUpdates(ctx, upstream, arch, channel, *v)
	if err != nil {
		logrus.Error(err)
	}

	return upgrade, upgrades, err
	//	// get latest channel version dowloaded

	//	// output a channel (struct) with the upgrade versions needed
	//
}

//func downloadRelease(c channel) error {
//	// Download the referneced versions by channel
//}

func (c Client) GetChannelLatest(ctx context.Context, uri *url.URL, arch string, channel string, version semver.Version) (Update, error) {
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
		return latest, &Error{Reason: "InvalidRequest", Message: err.Error(), cause: err}
	}
	req.Header.Add("Accept", GraphMediaType)
	if c.tlsConfig != nil {
		transport.TLSClientConfig = c.tlsConfig
	}

	if c.proxyURL != nil {
		transport.Proxy = http.ProxyURL(c.proxyURL)
	}

	client := http.Client{Transport: &transport}
	timeoutCtx, cancel := context.WithTimeout(ctx, getUpdatesTimeout)
	defer cancel()
	resp, err := client.Do(req.WithContext(timeoutCtx))
	if err != nil {
		return latest, &Error{Reason: "RemoteFailed", Message: err.Error(), cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return latest, &Error{Reason: "ResponseFailed", Message: fmt.Sprintf("unexpected HTTP status: %s", resp.Status)}
	}

	// Parse the graph.
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return latest, &Error{Reason: "ResponseFailed", Message: err.Error(), cause: err}
	}

	var graph graph
	if err = json.Unmarshal(body, &graph); err != nil {
		return latest, &Error{Reason: "ResponseInvalid", Message: err.Error(), cause: err}
	}

	// Find the current version within the graph.
	var currentIdx int
	found := false
	for i, node := range graph.Nodes {
		if version.EQ(node.Version) {
			currentIdx = i
			latest = Update(graph.Nodes[i])
			found = true
			break
		}
	}
	if !found {
		return latest, &Error{
			Reason:  "VersionNotFound",
			Message: fmt.Sprintf("currently reconciling cluster version %s not found in the %q channel", version, channel),
		}
	}
	logrus.Infoln(currentIdx)
	return latest, err
}

func GetReleases(i *Imageset, c *BundleSpec) error {
	//var nv []Update
	if i != nil {
		for _, r := range i.Mirror.Ocp.Channels {
			if r.Versions != nil {
				for _, rn := range r.Versions {
					rs, err := semver.New(rn)
					if err != nil {
						logrus.Errorln(err)
						return err
					}
					newest, neededVersions, err := calculateUpgradePath(r, rs)
					if err != nil {
						logrus.Errorln("Failed get upgrade graph")
						logrus.Error(err)
						return err
					}

					// If specific version, lookup version in channel and download

					// Else if channel, download latest from channel.

					// Use
					/*for _, u := range neededVersions {
											n := Update{}
											var buff bytes.Buffer
					                        enc :=gob.New
											e := json.Unmarshal(gob.NewDecoder(), n)
					*/

					logrus.Infof("Current Object: %v", rn)
					logrus.Infoln("")
					logrus.Infof("Newest: %v", newest)
					logrus.Infoln("")
					logrus.Infof("Next-Versions: %v", neededVersions)
					//nv = append(nv, neededVersions)
				}
			}
		}
	} else {
		for _, r := range c.Mirror.Ocp.Channels {
			for _, rn := range r.Versions {
				rs, err := semver.New(rn)
				if err != nil {
					logrus.Errorln(err)
					return err
				}
				newest, neededVersions, err := calculateUpgradePath(r, rs)
				if err != nil {
					logrus.Errorln("Failed get upgrade graph")
					logrus.Error(err)
					return err
				}

				// Use
				/*for _, u := range neededVersions {
				n := Update{}
				var buff bytes.Buffer
				enc :=gob.New
				e := json.Unmarshal(gob.NewDecoder(), n)
				*/

				logrus.Infof("No Meta Found. Requesting: %v", rn)
				logrus.Infoln("")
				logrus.Infof("Newest: %v", newest)
				logrus.Infoln("")
				logrus.Infof("Next-Versions: %v", neededVersions)
				//nv = append(nv, neededVersions)

			}
		}
	}
	return nil
	// Download each referenced version from
	//downloadRelease(nv)

}
