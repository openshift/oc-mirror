package mirror

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	semver "github.com/blang/semver/v4"
	"github.com/google/uuid"
	"github.com/openshift/oc/pkg/cli/admin/release"
	"github.com/sirupsen/logrus"

	"github.com/openshift/oc-mirror/pkg/cincinnati"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha1"
	"github.com/openshift/oc-mirror/pkg/image"
)

// ReleaseOptions configures either a Full or Diff mirror operation
// on a particular release image.
type ReleaseOptions struct {
	*MirrorOptions
	arch []string
	uuid uuid.UUID
	// insecure indicates whether the source
	// registry is insecure
	insecure bool
}

// NewReleaseOptions defaults ReleaseOptions.
func NewReleaseOptions(mo *MirrorOptions) *ReleaseOptions {
	relOpts := &ReleaseOptions{
		MirrorOptions: mo,
		arch:          mo.FilterOptions,
		uuid:          uuid.New(),
	}
	if mo.SourcePlainHTTP || mo.SourceSkipTLS {
		relOpts.insecure = true
	}
	return relOpts
}

// GetReleases will pill release payloads based on user configuration
func (o *ReleaseOptions) GetReleases(ctx context.Context, meta v1alpha1.Metadata, cfg *v1alpha1.ImageSetConfiguration) (image.AssociationSet, error) {

	var (
		srcDir           = filepath.Join(o.Dir, config.SourceDir)
		channelVersion   = make(map[string]string, len(cfg.Mirror.OCP.Channels))
		releaseDownloads = downloads{}
	)

	for _, ch := range cfg.Mirror.OCP.Channels {

		url := cincinnati.UpdateUrl
		if ch.Name == "okd" {
			url = cincinnati.OkdUpdateURL
		}

		client, upstream, err := cincinnati.NewClient(url, o.uuid)
		if err != nil {
			return nil, err
		}
		for _, arch := range o.arch {
			if len(ch.Versions) == 0 {
				// If no version was specified from the channel, then get the latest release
				latest, err := client.GetChannelLatest(ctx, upstream, arch, ch.Name)
				if err != nil {
					return nil, err
				}
				// Update version to release channel
				ch.Versions = append(ch.Versions, latest.String())
				channelVersion[ch.Name] = latest.String()
			}
			// Check for specific version declarations for each specific version
			for _, v := range ch.Versions {

				downloads, err := o.getDownloads(ctx, client, meta, v, ch.Name, arch, upstream)
				if err != nil {
					return nil, err
				}
				releaseDownloads.Merge(downloads)
			}
		}
	}

	assocs, err := o.mirror(ctx, srcDir, releaseDownloads)
	if err != nil {
		return nil, err
	}

	// Update cfg release channels with latest versions
	// if applicable
	cfg.Mirror.OCP.Channels = updateReleaseChannel(cfg.Mirror.OCP.Channels, channelVersion)

	return assocs, nil
}

// getDownloads will prepare the downloads map for mirroring
func (o *ReleaseOptions) getDownloads(ctx context.Context, client cincinnati.Client, meta v1alpha1.Metadata, version, channel, arch string, url *url.URL) (downloads, error) {
	downloads := map[string]download{}

	requested, err := semver.Parse(version)
	if err != nil {
		return nil, err
	}

	// If no release has been downloaded for the
	// channel, download the requested version
	lastCh, lastVer, err := cincinnati.FindLastRelease(meta, channel)
	currCh := channel
	reverse := false
	logrus.Infof("Downloading requested release %s", requested.String())
	switch {
	case err != nil && errors.Is(err, cincinnati.ErrNoPreviousRelease):
		lastVer = requested
		lastCh = channel
	case err != nil:
		return nil, err
	case requested.LT(lastVer):
		logrus.Debugf("Found current release %s", lastVer.String())
		// If the requested version is an earlier release than previous
		// downloads switch the values to get updates between the
		// later and earlier version
		currCh = lastCh
		lastCh = channel
		requested = lastVer
		lastVer = semver.MustParse(version)
		// Download the current image since this will not be in the updates
		reverse = true
	default:
		logrus.Debugf("Found current release %s", lastVer.String())
	}

	// This dumps the available upgrades from the last downloaded version
	current, new, updates, err := client.CalculateUpgrades(ctx, url, arch, lastCh, currCh, lastVer, requested)
	if err != nil {
		return nil, fmt.Errorf("failed to get upgrade graph: %v", err)
	}

	for _, update := range updates {
		download := download{
			Update: update,
			arch:   arch,
		}
		downloads[update.Image] = download
	}

	// If reverse graph download the current version
	// else add new to downloads
	if reverse {
		download := download{
			Update: current,
			arch:   arch,
		}
		downloads[current.Image] = download
		// Remove new from updates as it has already
		// been downloaded
		delete(downloads, new.Image)
	} else {
		download := download{
			Update: new,
			arch:   arch,
		}
		downloads[new.Image] = download
	}

	return downloads, nil
}

// mirror will take the prepared download information and mirror to disk location
func (o *ReleaseOptions) mirror(ctx context.Context, toDir string, downloads map[string]download) (image.AssociationSet, error) {
	allAssocs := image.AssociationSet{}

	for img, download := range downloads {
		logrus.Debugf("Starting release download for version %s", download.Version.String())
		opts := release.NewMirrorOptions(o.IOStreams)
		opts.ToDir = toDir

		regctx, err := config.CreateDefaultContext(o.insecure)
		if err != nil {
			return nil, fmt.Errorf("error creating registry context: %v", err)
		}
		opts.SecurityOptions.CachedContext = regctx

		opts.SecurityOptions.Insecure = o.insecure
		opts.SecurityOptions.SkipVerification = o.SkipVerification
		opts.DryRun = o.DryRun
		opts.From = img
		if err := opts.Validate(); err != nil {
			return nil, err
		}

		// Create release mapping and get images list
		// before mirroring actions
		mappings, images, err := o.getMapping(ctx, *opts)
		if err != nil {
			return nil, fmt.Errorf("error retrieving mapping information for %s: %v", img, err)
		}

		// Complete mirroring actions
		if err := opts.Run(); err != nil {
			return nil, err
		}

		// Do not build associations on dry runs because there are no manifests
		if !o.DryRun {

			logrus.Debugln("starting image association")
			assocs, err := image.AssociateImageLayers(toDir, mappings, images, image.TypeOCPRelease)
			if err != nil {
				return nil, err
			}

			allAssocs.Merge(assocs)
		}
	}

	return allAssocs, nil
}

// getMapping will run release mirror with ToMirror set to true to get mapping information
func (o *ReleaseOptions) getMapping(ctx context.Context, opts release.MirrorOptions) (map[string]string, []string, error) {

	mappingPath := filepath.Join(o.Dir, mappingFile)
	file, err := os.Create(mappingPath)
	defer os.Remove(mappingPath)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	opts.IOStreams.Out = file
	opts.ToMirror = true

	if err := opts.Validate(); err != nil {
		return nil, nil, err
	}
	if err := opts.Run(); err != nil {
		return nil, nil, err
	}

	mappings, err := image.ReadImageMapping(mappingPath, " ")
	if err != nil {
		return nil, nil, err
	}
	var images []string
	for img, _ := range mappings {
		images = append(images, img)
	}

	return mappings, images, nil
}

// updateReleaseChannel will add a version to the ReleaseChannel to record
// for metadata
func updateReleaseChannel(releaseChannels []v1alpha1.ReleaseChannel, channelVersions map[string]string) []v1alpha1.ReleaseChannel {
	for i, ch := range releaseChannels {
		v, found := channelVersions[ch.Name]
		if found {
			releaseChannels[i].Versions = append(releaseChannels[i].Versions, v)
		}
	}
	return releaseChannels
}

// Define download types
type downloads map[string]download
type download struct {
	cincinnati.Update
	arch string
}

func (d downloads) Merge(in downloads) {
	for k, v := range in {
		d[k] = v
	}
}
