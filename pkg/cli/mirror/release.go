package mirror

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	semver "github.com/blang/semver/v4"
	"github.com/google/uuid"
	"github.com/openshift/oc/pkg/cli/admin/release"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"

	"github.com/RedHatGov/bundle/pkg/bundle"
	"github.com/RedHatGov/bundle/pkg/cincinnati"
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
	MirrorOptions
	release string
	arch    []string
	uuid    uuid.UUID
}

// NewReleaseOptions defaults ReleaseOptions.
func NewReleaseOptions(mo MirrorOptions, flags *pflag.FlagSet) *ReleaseOptions {
	var arch []string
	opts := mo.FilterOptions
	opts.Complete(flags)
	if opts.IsWildcardFilter() {
		arch = supportedArchs
	} else {
		arch = []string{strings.Join(strings.Split(opts.FilterByOS, "/")[1:], "/")}
	}

	return &ReleaseOptions{
		MirrorOptions: mo,
		arch:          arch,
		uuid:          uuid.New(),
	}
}

func (o *ReleaseOptions) downloadMirror(secret []byte, toDir, from, arch, version string) (image.AssociationSet, error) {
	opts := release.NewMirrorOptions(o.IOStreams)
	opts.From = from
	opts.ToDir = toDir

	// If the pullSecret is not empty create a cached context
	// else let `oc mirror` use the default docker config location
	if len(secret) != 0 {
		ctx, err := config.CreateContext(secret, o.SkipVerification, o.SourceSkipTLS)
		if err != nil {
			return nil, err
		}
		opts.SecurityOptions.CachedContext = ctx
	}

	opts.SecurityOptions.Insecure = o.SourceSkipTLS
	opts.SecurityOptions.SkipVerification = o.SkipVerification
	opts.DryRun = o.DryRun
	logrus.Debugf("Starting release download for version %s", version)
	if err := opts.Run(); err != nil {
		return nil, err
	}

	// Retrive the mapping information for release
	mapping, images, err := o.getMapping(*opts, arch, version)

	if err != nil {
		return nil, fmt.Errorf("error could not retrieve mapping information: %v", err)
	}

	logrus.Debugln("starting image association")
	assocs, err := image.AssociateImageLayers(toDir, mapping, images, image.TypeOCPRelease)
	if err != nil {
		return nil, err
	}

	// Check if a release image was provided with mapping
	if o.release == "" {
		return nil, errors.New("release image not found in mapping")
	}

	// Update all images associated with a release to the
	// release images so they form one keyset for publising
	for _, img := range images {
		assocs.UpdateKey(img, o.release)
	}

	return assocs, nil
}

// GetReleases will pill release payloads based on user configuration
func (o *ReleaseOptions) GetReleases(ctx context.Context, meta v1alpha1.Metadata, cfg *v1alpha1.ImageSetConfiguration) (image.AssociationSet, error) {

	allAssocs := image.AssociationSet{}
	pullSecret := cfg.Mirror.OCP.PullSecret
	srcDir := filepath.Join(o.Dir, config.SourceDir)
	channelVersion := make(map[string]string, len(cfg.Mirror.OCP.Channels))

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

				requested, err := semver.Parse(v)
				if err != nil {
					return nil, err
				}

				// If no release has been downloaded for the
				// channel, download the requested version
				lastCh, lastVer, err := cincinnati.FindLastRelease(meta, ch.Name, url, o.uuid)
				logrus.Infof("Downloading requested release %s", requested.String())
				switch {
				case err != nil && errors.As(err, &cincinnati.ErrNoPreviousRelease):
					lastVer = requested
				case err != nil:
					return nil, err
				case requested.LT(lastVer):
					// If the requested version is a earlier release than previous
					// downloads switch the values to get updates between the
					// later and earlier version
					lastVer = requested
					requested = lastVer
				default:
					logrus.Debugf("Found current release %s", lastVer.String())
				}

				// This dumps the available upgrades from the last downloaded version
				current, new, updates, err := client.CalculateUpgrades(ctx, upstream, arch, lastCh, ch.Name, lastVer, requested)
				if err != nil {
					return nil, fmt.Errorf("failed to get upgrade graph: %v", err)
				}

				if requested.EQ(lastVer) {
					assocs, err := o.downloadMirror([]byte(pullSecret), srcDir, current.Image, arch, v)
					if err != nil {
						return nil, err
					}
					allAssocs.Merge(assocs)
					continue
				}

				// Download needed version between the current version and
				// the requested version.
				var updateCount int
				for _, update := range updates {
					logrus.Debugf("Image to download for: %v", update.Image)
					if update.Version.LE(requested) {
						assocs, err := o.downloadMirror([]byte(pullSecret), srcDir, update.Image, arch, update.Version.String())
						if err != nil {
							return nil, err
						}
						allAssocs.Merge(assocs)
						updateCount++
					}
				}
				// Downloaded requested release if not in upgrade
				// graph
				if updateCount == 0 {
					assocs, err := o.downloadMirror([]byte(pullSecret), srcDir, new.Image, arch, new.Version.String())
					if err != nil {
						return nil, err
					}
					allAssocs.Merge(assocs)
				}
			}
		}
	}

	// Update cfg release channels with latest versions
	// if applicable
	cfg.Mirror.OCP.Channels = updateReleaseChannel(cfg.Mirror.OCP.Channels, channelVersion)

	return allAssocs, nil
}

// getMapping will run release mirror with ToMirror set to true to get mapping information
func (o *ReleaseOptions) getMapping(opts release.MirrorOptions, arch, version string) (mappings map[string]string, images []string, err error) {

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
		idx := strings.LastIndex(text, " ")
		if idx == -1 {
			return nil, nil, fmt.Errorf("invalid mapping information for release %v", version)
		}
		srcRef := text[:idx]
		// Get release image name from mapping
		// Only the top release need to be resolve because all other image key associated to the
		// will be updated to this value
		//
		// afflom - Select on ocp-release OR origin
		if strings.Contains(srcRef, "ocp-release") || strings.Contains(srcRef, "origin/release") {
			if !image.IsImagePinned(srcRef) {
				srcRef, err = bundle.PinImages(context.TODO(), srcRef, "", o.SourceSkipTLS)
			}
			o.release = srcRef
		}

		// Generate name of target directory
		dstRef := opts.TargetFn(text[idx+1:]).Exact()
		nameIdx := strings.LastIndex(dstRef, version)
		if nameIdx == -1 {
			return nil, nil, fmt.Errorf("image missing version %s for image %q", version, srcRef)
		}
		image := dstRef[nameIdx+len(version):]
		image = strings.TrimPrefix(image, "-")
		names := []string{version, arch}
		if image != "" {
			names = append(names, image)
		}
		dstRef = strings.Join(names, "-")

		// Append mapping file
		if _, err := file.WriteString(srcRef + "=file://openshift/release:" + dstRef + "\n"); err != nil {
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
