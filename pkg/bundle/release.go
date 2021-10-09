package bundle

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	semver "github.com/blang/semver/v4"
	"github.com/google/uuid"
	"github.com/openshift/oc/pkg/cli/admin/release"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"

	"github.com/RedHatGov/bundle/pkg/cli"
	"github.com/RedHatGov/bundle/pkg/config"
	"github.com/RedHatGov/bundle/pkg/config/v1alpha1"
	"github.com/RedHatGov/bundle/pkg/image"
)

var supportedArchs = []string{"amd64", "ppc64le", "s390x"}

// archMap maps Go architecture strings to OpenShift supported values for any that differ.
var archMap = map[string]string{
	"amd64": "x86_64",
}

// ReleaseOptions configures either a Full or Diff mirror operation
// on a particular release image.
type ReleaseOptions struct {
	cli.RootOptions
	release string
	arch    []string
}

// NewReleaseOptions defaults ReleaseOptions.
func NewReleaseOptions(ro cli.RootOptions, flags *pflag.FlagSet) *ReleaseOptions {
	var arch []string
	opts := ro.FilterOptions
	opts.Complete(flags)
	if opts.IsWildcardFilter() {
		arch = supportedArchs
	} else {
		arch = []string{strings.Join(strings.Split(opts.FilterByOS, "/")[1:], "/")}
	}

	return &ReleaseOptions{
		RootOptions: ro,
		arch:        arch,
	}
}

const (
	UpdateUrl    string = "https://api.openshift.com/api/upgrades_info/v1/graph"
	OkdUpdateURL string = "https://origin-release.ci.openshift.org/graph"
)

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

func newClient(u string) (Client, *url.URL, error) {

	upstream, err := url.Parse(u)
	if err != nil {
		return Client{}, nil, err
	}

	// This needs to handle user input at some point.
	//var proxy *url.URL

	tls, err := getTLSConfig()
	if err != nil {
		return Client{}, nil, err
	}

	transport := &http.Transport{
		TLSClientConfig: tls,
	}
	return NewClient(uuid.New(), transport), upstream, nil
}

// Next calculate the upgrade path from the current version to the channel's latest
func calculateUpgradePath(ch v1alpha1.ReleaseChannel, v semver.Version, url, arch string) (Update, []Update, error) {

	client, upstream, err := newClient(url)
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
}

func GetLatestVersion(ch v1alpha1.ReleaseChannel, url, arch string) (Update, error) {

	client, upstream, err := newClient(url)
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
}

func (o *ReleaseOptions) downloadMirror(secret []byte, toDir, from, arch string) (image.AssociationSet, error) {
	opts := release.NewMirrorOptions(o.IOStreams)
	opts.From = from
	opts.ToDir = toDir

	// If the pullSecret is not empty create a cached context
	// else let `oc mirror` use the default docker config location
	if len(secret) != 0 {
		ctx, err := config.CreateContext(secret, o.SkipVerification, o.SkipTLS)
		if err != nil {
			return nil, err
		}
		opts.SecurityOptions.CachedContext = ctx
	}

	opts.SecurityOptions.Insecure = o.SkipTLS
	opts.SecurityOptions.SkipVerification = o.SkipVerification
	opts.DryRun = o.DryRun
	logrus.Infoln("Starting release download")
	if err := opts.Run(); err != nil {
		return nil, err
	}

	// Retrive the mapping information for release
	logrus.Infoln("starting mapping")
	mapping, images, err := o.getMapping(*opts, arch)

	logrus.Info(mapping)

	if err != nil {
		return nil, fmt.Errorf("error could retrieve mapping information: %v", err)
	}

	logrus.Infoln("starting image association")
	assocs, err := image.AssociateImageLayers(toDir, mapping, images, image.TypeOCPRelease)
	if err != nil {
		return nil, err
	}

	// Check if a release image was provided with mapping
	if o.release == "" {
		logrus.Infoln("whats new?")
		return nil, errors.New("release image not found in mapping")
	}

	// Update all images associated with a release to the
	// release images so they form one keyset for publising
	for _, img := range images {
		logrus.Infoln("whats a key?")
		assocs.UpdateKey(img, o.release)
	}

	return assocs, nil
}

func (o *ReleaseOptions) GetReleasesInitial(cfg v1alpha1.ImageSetConfiguration) (image.AssociationSet, error) {

	allAssocs := image.AssociationSet{}
	pullSecret := cfg.Mirror.OCP.PullSecret
	srcDir := filepath.Join(o.Dir, config.SourceDir)

	// For each channel in the config file
	for _, ch := range cfg.Mirror.OCP.Channels {
		var url string
		if ch.Name == "okd" {
			url = OkdUpdateURL
		} else {
			url = UpdateUrl
		}
		for _, arch := range o.arch {

			if len(ch.Versions) == 0 {

				// If no version was specified from the channel, then get the latest release
				latest, err := GetLatestVersion(ch, url, arch)
				if err != nil {
					return nil, err
				}
				logrus.Infof("Image to download: %v", latest.Image)
				// Download the release
				assocs, err := o.downloadMirror([]byte(pullSecret), srcDir, latest.Image, arch)
				if err != nil {
					return nil, err
				}
				logrus.Infoln("release, before merge")
				allAssocs.Merge(assocs)
				logrus.Infof("Channel Latest version %v", latest.Version)
			}

			// Check for specific version declarations for each specific version
			for _, v := range ch.Versions {

				// Convert the string to a semver
				ver, err := semver.Parse(v)

				if err != nil {
					return nil, err
				}

				// This dumps the available upgrades from the last downloaded version
				requested, _, err := calculateUpgradePath(ch, ver, url, arch)
				if err != nil {
					return nil, fmt.Errorf("failed to get upgrade graph: %v", err)
				}

				logrus.Infof("requested: %v", requested.Version)
				assocs, err := o.downloadMirror([]byte(pullSecret), srcDir, requested.Image, arch)
				if err != nil {
					return nil, err
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
	}

	// Download each referenced version from
	//downloadRelease(nv)

	return allAssocs, nil
}

func (o *ReleaseOptions) GetReleasesDiff(_ v1alpha1.PastMirror, cfg v1alpha1.ImageSetConfiguration) (image.AssociationSet, error) {

	allAssocs := image.AssociationSet{}
	pullSecret := cfg.Mirror.OCP.PullSecret
	srcDir := filepath.Join(o.Dir, config.SourceDir)

	for _, ch := range cfg.Mirror.OCP.Channels {
		var url string
		if ch.Name == "okd" {
			url = OkdUpdateURL
		} else {
			url = UpdateUrl
		}
		for _, arch := range o.arch {
			// Check for specific version declarations for each specific version
			for _, v := range ch.Versions {

				// Convert the string to a semver
				ver, err := semver.Parse(v)

				if err != nil {
					return nil, err
				}

				// This dumps the available upgrades from the last downloaded version
				requested, _, err := calculateUpgradePath(ch, ver, url, arch)
				if err != nil {
					return nil, fmt.Errorf("failed to get upgrade graph: %v", err)
				}

				logrus.Infof("requested: %v", requested.Version)
				assocs, err := o.downloadMirror([]byte(pullSecret), srcDir, requested.Image, arch)
				if err != nil {
					return nil, err
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
	}

	// Download each referenced version from
	//downloadRelease(nv)

	return allAssocs, nil
}

// getMapping will run release mirror with ToMirror set to true to get mapping information
// FIXME(jpower): the provided mapping does not provide a digest mapping so ICSP cannot
// be created
func (o *ReleaseOptions) getMapping(opts release.MirrorOptions, arch string) (mappings map[string]string, images []string, err error) {

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

	newArch, found := archMap[arch]
	if found {
		arch = newArch
	}

	scanner := bufio.NewScanner(&buffer)

	// Scan mapping output and write to file
	for scanner.Scan() {
		text := scanner.Text()
		split := strings.Split(text, " ")
		srcRef := split[0]

		// Get release image name from mapping
		// Only the top release need to be resolve because all other image key associated to the
		// will be updated to this value
		if strings.Contains(srcRef, "ocp-release") {
			if !image.IsImagePinned(srcRef) {
				srcRef, err = pinImages(context.TODO(), srcRef, "", o.SkipTLS)
			}
			o.release = srcRef
		}

		// Generate name of target directory
		var names []string
		dstRef := opts.TargetFn(split[1]).Exact()

		// TODO: arch is not provided when getting mapping from release does not included
		// the arch in the directory. Need to investigate, but adding it here as a workaround
		nameSplit := strings.Split(dstRef, "-")
		names = []string{nameSplit[1], arch}
		names = append(names, nameSplit[2:]...)

		dstRef = strings.Join(names, "-")

		if _, err := file.WriteString(srcRef + "=" + dstRef + "\n"); err != nil {
			return mappings, images, err
		}

		images = append(images, srcRef)
	}

	mappings, err = image.ReadImageMapping(mappingPath)

	if err != nil {
		return mappings, images, err
	}

	return mappings, images, nil
}
