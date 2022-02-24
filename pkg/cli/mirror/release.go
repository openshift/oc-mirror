package mirror

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	semver "github.com/blang/semver/v4"
	"github.com/google/uuid"
	"github.com/openshift/oc/pkg/cli/admin/release"
	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/openshift/oc-mirror/pkg/cincinnati"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha2"
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
	url      string
}

// TODO(jpower432): replace OKD download support

// NewReleaseOptions defaults ReleaseOptions.
func NewReleaseOptions(mo *MirrorOptions) *ReleaseOptions {
	relOpts := &ReleaseOptions{
		MirrorOptions: mo,
		arch:          mo.FilterOptions,
		uuid:          uuid.New(),
		url:           cincinnati.UpdateUrl,
	}
	if mo.SourcePlainHTTP || mo.SourceSkipTLS {
		relOpts.insecure = true
	}
	return relOpts
}

// Plan will pill release payloads based on user configuration
func (o *ReleaseOptions) Plan(ctx context.Context, lastRun v1alpha2.PastMirror, cfg *v1alpha2.ImageSetConfiguration) (image.TypedImageMapping, error) {

	var (
		srcDir           = filepath.Join(o.Dir, config.SourceDir)
		releaseDownloads = downloads{}
		mmapping         = image.TypedImageMapping{}
		errs             = []error{}
	)

	client, upstream, err := cincinnati.NewClient(o.url, o.uuid)
	if err != nil {
		return mmapping, err
	}
	for _, arch := range o.arch {

		channelVersion := make(map[string]string, len(cfg.Mirror.OCP.Channels))

		for _, ch := range cfg.Mirror.OCP.Channels {

			if len(ch.MaxVersion) == 0 && len(ch.MinVersion) == 0 {
				// If no version was specified from the channel, then get the latest release
				latest, err := client.GetChannelLatest(ctx, upstream, arch, ch.Name)
				if err != nil {
					errs = append(errs, err)
					continue
				}
				// Update version to release channel
				ch.MaxVersion = latest.String()
				ch.MinVersion = latest.String()
				channelVersion[ch.Name] = latest.String()
			}

			downloads, err := o.getChannelDownloads(ctx, client, lastRun.Mirror.OCP.Channels, ch, arch, upstream)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			releaseDownloads.Merge(downloads)
		}

		// Update cfg release channels with maximum versions
		// if applicable
		cfg.Mirror.OCP.Channels = updateReleaseChannel(cfg.Mirror.OCP.Channels, channelVersion)

		// Get cross-channel updates
		// TODO(jpower432): Record blocked edges
		firstCh, first, err := cincinnati.FindRelease(cfg.Mirror, true)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		lastCh, last, err := cincinnati.FindRelease(cfg.Mirror, false)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		current, newest, updates, err := client.CalculateUpgrades(ctx, upstream, arch, firstCh, lastCh, first, last)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get upgrade graph: %v", err))
			continue
		}
		newDownloads := gatherUpdates(current, newest, updates)
		releaseDownloads.Merge(newDownloads)
	}
	if len(errs) != 0 {
		return mmapping, utilerrors.NewAggregate(errs)
	}

	opts, err := o.newMirrorReleaseOptions(srcDir)
	if err != nil {
		return mmapping, err
	}

	for img := range releaseDownloads {
		logrus.Debugf("Starting release download for version %s", img)
		opts.From = img

		// Create release mapping and get images list
		// before mirroring actions
		mappings, err := o.getMapping(*opts)
		if err != nil {
			return mmapping, fmt.Errorf("error retrieving mapping information for %s: %v", img, err)
		}
		mmapping.Merge(mappings)
	}

	return mmapping, nil
}

// getDownloads will prepare the downloads map for mirroring
func (o *ReleaseOptions) getChannelDownloads(ctx context.Context, client cincinnati.Client, lastChannels []v1alpha2.ReleaseChannel, channel v1alpha2.ReleaseChannel, arch string, url *url.URL) (downloads, error) {
	allDownloads := downloads{}

	var prevChannel v1alpha2.ReleaseChannel
	for _, ch := range lastChannels {
		if ch.Name == channel.Name {
			prevChannel = ch
		}
	}

	if prevChannel.Name != "" {
		// If the requested min version is less than the previous, add downloads
		if prevChannel.MinVersion > channel.MinVersion {
			first, err := semver.Parse(channel.MinVersion)
			if err != nil {
				return allDownloads, err
			}
			last, err := semver.Parse(prevChannel.MinVersion)
			if err != nil {
				return allDownloads, err
			}
			current, newest, updates, err := client.CalculateUpgrades(ctx, url, arch, channel.Name, channel.Name, first, last)
			if err != nil {
				return allDownloads, err
			}
			newDownloads := gatherUpdates(current, newest, updates)
			allDownloads.Merge(newDownloads)
		}

		// If the requested max version is more than the previous, add downloads
		if prevChannel.MaxVersion < channel.MaxVersion {
			first, err := semver.Parse(prevChannel.MaxVersion)
			if err != nil {
				return allDownloads, err
			}
			last, err := semver.Parse(channel.MinVersion)
			if err != nil {
				return allDownloads, err
			}
			current, newest, updates, err := client.CalculateUpgrades(ctx, url, arch, channel.Name, channel.Name, first, last)
			if err != nil {
				return allDownloads, err
			}
			newDownloads := gatherUpdates(current, newest, updates)
			allDownloads.Merge(newDownloads)
		}
	}

	// Plot between min and max of channel
	first, err := semver.Parse(channel.MinVersion)
	if err != nil {
		return allDownloads, err
	}
	last, err := semver.Parse(channel.MaxVersion)
	if err != nil {
		return allDownloads, err
	}
	current, newest, updates, err := client.GetUpdates(ctx, url, arch, channel.Name, first, last)
	if err != nil {
		return allDownloads, err
	}
	newDownloads := gatherUpdates(current, newest, updates)
	allDownloads.Merge(newDownloads)

	return allDownloads, nil
}

func gatherUpdates(current, newest cincinnati.Update, updates []cincinnati.Update) downloads {
	releaseDownloads := downloads{}
	for _, update := range updates {
		releaseDownloads[update.Image] = struct{}{}
	}

	releaseDownloads[current.Image] = struct{}{}
	releaseDownloads[newest.Image] = struct{}{}
	return releaseDownloads
}

func (o *ReleaseOptions) newMirrorReleaseOptions(fileDir string) (*release.MirrorOptions, error) {
	opts := release.NewMirrorOptions(o.IOStreams)
	opts.DryRun = o.DryRun
	opts.ToDir = fileDir

	opts.SecurityOptions.Insecure = o.insecure
	opts.SecurityOptions.SkipVerification = o.SkipVerification

	regctx, err := config.CreateDefaultContext(o.insecure)
	if err != nil {
		return nil, fmt.Errorf("error creating registry context: %v", err)
	}
	opts.SecurityOptions.CachedContext = regctx

	return opts, nil
}

// getMapping will run release mirror with ToMirror set to true to get mapping information
func (o *ReleaseOptions) getMapping(opts release.MirrorOptions) (image.TypedImageMapping, error) {
	mappingPath := filepath.Join(o.Dir, mappingFile)
	file, err := os.Create(mappingPath)
	defer os.Remove(mappingPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	opts.IOStreams.Out = file
	opts.ToMirror = true

	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if err := opts.Run(); err != nil {
		return nil, err
	}

	mappings, err := image.ReadImageMapping(mappingPath, " ", image.TypeOCPRelease)
	if err != nil {
		return nil, err
	}

	return mappings, nil
}

// updateReleaseChannel will add a version to the ReleaseChannel to record
// for metadata
func updateReleaseChannel(releaseChannels []v1alpha2.ReleaseChannel, channelVersions map[string]string) []v1alpha2.ReleaseChannel {
	for i, ch := range releaseChannels {
		v, found := channelVersions[ch.Name]
		if found {
			releaseChannels[i].MaxVersion = v
			releaseChannels[i].MinVersion = v
		}
	}
	return releaseChannels
}

// Define download types
type downloads map[string]struct{}

func (d downloads) Merge(in downloads) {
	for k, v := range in {
		_, ok := d[k]
		if ok {
			logrus.Debugf("download %s exists", k)
			continue
		}
		d[k] = v
	}
}
