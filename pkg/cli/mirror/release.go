package mirror

import (
	"context"
	"fmt"
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

const (
	releaseRepo = "release-images"
)

// ReleaseOptions configures either a Full or Diff mirror operation
// on a particular release image.
type ReleaseOptions struct {
	*MirrorOptions
	arch []string
	// insecure indicates whether the source
	// registry is insecure
	insecure bool
	uuid     uuid.UUID
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

// Plan will pill release payloads based on user configuration
func (o *ReleaseOptions) Plan(ctx context.Context, lastRun v1alpha2.PastMirror, cfg *v1alpha2.ImageSetConfiguration) (image.TypedImageMapping, error) {

	var (
		srcDir           = filepath.Join(o.Dir, config.SourceDir)
		releaseDownloads = downloads{}
		mmapping         = image.TypedImageMapping{}
		errs             = []error{}
	)

	for _, arch := range o.arch {

		versionsByChannel := make(map[string]v1alpha2.ReleaseChannel, len(cfg.Mirror.OCP.Channels))

		for _, ch := range cfg.Mirror.OCP.Channels {

			var client cincinnati.Client
			var err error
			if ch.Name == cincinnati.OkdChannel {
				client, err = cincinnati.NewOKDClient(o.uuid)
			} else {
				client, err = cincinnati.NewOCPClient(o.uuid)
			}
			if err != nil {
				errs = append(errs, err)
				continue
			}

			if len(ch.MaxVersion) == 0 || len(ch.MinVersion) == 0 {

				// Find channel maximum value and only set the minimum as well if heads-only is true
				if len(ch.MaxVersion) == 0 {
					latest, err := cincinnati.GetChannelMinOrMax(ctx, client, arch, ch.Name, false)
					if err != nil {
						errs = append(errs, err)
						continue
					}

					// Update version to release channel
					ch.MaxVersion = latest.String()
					if len(ch.MinVersion) == 0 && ch.IsHeadsOnly() {
						ch.MinVersion = latest.String()
					}
				}

				// Find channel minimum if heads-only is false or just the minimum is not set
				// in the config
				if len(ch.MinVersion) == 0 {
					first, err := cincinnati.GetChannelMinOrMax(ctx, client, arch, ch.Name, true)
					if err != nil {
						errs = append(errs, err)
						continue
					}
					ch.MinVersion = first.String()
				}
				versionsByChannel[ch.Name] = ch
			}

			downloads, err := o.getChannelDownloads(ctx, client, lastRun.Mirror.OCP.Channels, ch, arch)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			releaseDownloads.Merge(downloads)
		}

		// Update cfg release channels with maximum and minimum versions
		// if applicable
		cfg.Mirror.OCP.Channels = updateReleaseChannel(cfg.Mirror.OCP.Channels, versionsByChannel)

		if len(cfg.Mirror.OCP.Channels) > 1 {
			newDownloads, err := o.getCrossChannelDownloads(ctx, arch, cfg.Mirror.OCP.Channels)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			releaseDownloads.Merge(newDownloads)
		}
	}
	if len(errs) != 0 {
		return mmapping, utilerrors.NewAggregate(errs)
	}

	for img := range releaseDownloads {
		logrus.Debugf("Starting release download for version %s", img)
		opts, err := o.newMirrorReleaseOptions(srcDir)
		if err != nil {
			return mmapping, err
		}
		opts.From = img

		// Create release mapping and get images list
		// before mirroring actions
		mappings, err := o.getMapping(opts)
		if err != nil {
			return mmapping, fmt.Errorf("error retrieving mapping information for %s: %v", img, err)
		}
		mmapping.Merge(mappings)
	}

	return mmapping, nil
}

// getDownloads will prepare the downloads map for mirroring
func (o *ReleaseOptions) getChannelDownloads(ctx context.Context, c cincinnati.Client, lastChannels []v1alpha2.ReleaseChannel, channel v1alpha2.ReleaseChannel, arch string) (downloads, error) {
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
			current, newest, updates, err := cincinnati.CalculateUpgrades(ctx, c, arch, channel.Name, channel.Name, first, last)
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
			current, newest, updates, err := cincinnati.CalculateUpgrades(ctx, c, arch, channel.Name, channel.Name, first, last)
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
	current, newest, updates, err := cincinnati.CalculateUpgrades(ctx, c, arch, channel.Name, channel.Name, first, last)
	if err != nil {
		return allDownloads, err
	}
	newDownloads := gatherUpdates(current, newest, updates)
	allDownloads.Merge(newDownloads)

	return allDownloads, nil
}

// getCrossChannelDownloads will determine required downloads between channel versions (for OCP only)
func (o *ReleaseOptions) getCrossChannelDownloads(ctx context.Context, arch string, channels []v1alpha2.ReleaseChannel) (downloads, error) {
	// Strip any OKD channels from the list
	ocpChannels := make([]v1alpha2.ReleaseChannel, len(channels))
	copy(ocpChannels, channels)
	for i, ch := range ocpChannels {
		if ch.Name == cincinnati.OkdChannel {
			ocpChannels = append(ocpChannels[:i], ocpChannels[i+1:]...)
		}
	}
	// If no other channels exist, return no downloads
	if len(ocpChannels) == 0 {
		return downloads{}, nil
	}
	client, err := cincinnati.NewOCPClient(o.uuid)
	if err != nil {
		return downloads{}, err
	}

	firstCh, first, err := cincinnati.FindRelease(ocpChannels, true)
	if err != nil {
		return downloads{}, fmt.Errorf("failed to find minimum release version: %v", err)
	}
	lastCh, last, err := cincinnati.FindRelease(ocpChannels, false)
	if err != nil {
		return downloads{}, fmt.Errorf("failed to find maximum release version: %v", err)
	}
	current, newest, updates, err := cincinnati.CalculateUpgrades(ctx, client, arch, firstCh, lastCh, first, last)
	if err != nil {
		return downloads{}, fmt.Errorf("failed to get upgrade graph: %v", err)
	}
	return gatherUpdates(current, newest, updates), nil
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

	regctx, err := image.CreateDefaultContext(o.insecure)
	if err != nil {
		return nil, fmt.Errorf("error creating registry context: %v", err)
	}
	opts.SecurityOptions.CachedContext = regctx

	return opts, nil
}

// getMapping will run release mirror with ToMirror set to true to get mapping information
func (o *ReleaseOptions) getMapping(opts *release.MirrorOptions) (image.TypedImageMapping, error) {
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

	releaseImageRef, err := image.ParseTypedImage(opts.From, image.TypeOCPRelease)
	if err != nil {
		return nil, err
	}
	dstReleaseRef, ok := mappings[releaseImageRef]
	if !ok {
		return nil, fmt.Errorf("release images %s not found in mapping", opts.From)
	}
	dstReleaseRef.Ref.Name = releaseRepo
	mappings[releaseImageRef] = dstReleaseRef

	return mappings, nil
}

// updateReleaseChannel will add a version to the ReleaseChannel to record
// for metadata
func updateReleaseChannel(releaseChannels []v1alpha2.ReleaseChannel, versionsByKey map[string]v1alpha2.ReleaseChannel) []v1alpha2.ReleaseChannel {
	for i, ch := range releaseChannels {
		ch, found := versionsByKey[ch.Name]
		if found {
			releaseChannels[i] = ch
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
