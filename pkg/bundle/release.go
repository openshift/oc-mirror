package bundle

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"

	"github.com/blang/semver/v4"
	"github.com/google/uuid"
	cincinnati "github.com/openshift/cluster-version-operator/pkg/cincinnati"
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
func (c channel) calculateUpgradePath(b *BundleSpec) (cincinnati.Update, []cincinnati.Update, error) {

	upstream, err := url.Parse("https://api.openshift.com/api/upgrades_info/v1/graph")
	if err != nil {
		logrus.Error(err)
	}
	proxy, err := url.Parse("")
	if err != nil {
		logrus.Error(err)
	}

	tls, err := getTLSConfig()
	client := cincinnati.NewClient(uuid.New(), proxy, tls)

	ctx := context.Background()
	arch := "x86_64"

	channel := "stable-4.7"

	var version = semver.Version{
		Major: 4,
		Minor: 7,
		Patch: 3,
	}

	upgrade, upgrades, err := client.GetUpdates(ctx, upstream, arch, channel, version)
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

func GetReleases(i *Imagesets, rootDir string) error {
	// Get latest downloaded versions by channel
	// Get bundle config release info
	config, err := readBundleConfig(rootDir)
	if err != nil {
		logrus.Errorln("Failed to load config file")
		logrus.Error(err)
		return err
	}
	if i != nil {
		var lastVersions Metadata
		lastVersions = i.Imagesets[len(i.Imagesets)-1]
		for _, r := range lastVersions.Ocp.Channels {
			var nv channels
			newest, neededVersions, err := r.calculateUpgradePath(config)
			if err != nil {
				logrus.Errorln("Failed get upgrade graph")
				logrus.Error(err)
				return err
			}
			nv = append(nv, neededVersions)
		}

		// Download each referenced version from
		//downloadRelease(nv)
	} else {
		// download release speficied in imageset
	}

	return err

}
