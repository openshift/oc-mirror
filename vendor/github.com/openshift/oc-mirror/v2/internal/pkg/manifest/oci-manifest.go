package manifest

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	digest "github.com/opencontainers/go-digest"

	"github.com/blang/semver/v4"
	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/otiai10/copy"
	"k8s.io/klog/v2"
)

type OperatorCatalog struct {
	// Packages is a map that stores the packages in the operator catalog.
	// The key is the package name and the value is the corresponding declcfg.Package object.
	Packages map[string]declcfg.Package
	// Channels is a map that stores the channels for each package in the operator catalog.
	// The key is the package name and the value is a slice of declcfg.Channel objects.
	Channels map[string][]declcfg.Channel
	// ChannelEntries is a map that stores the channel entries (Bundle names) for each channel and package in the operator catalog.
	// The first key is the package name, the second key is the channel name, and the third key is the bundle name (or channel entry name).
	// The value is the corresponding declcfg.ChannelEntry object.
	ChannelEntries map[string]map[string]map[string]declcfg.ChannelEntry
	// BundlesByPkgAndName is a map that stores the bundles for each package and bundle name in the operator catalog.
	// The first key is the package name, the second key is the bundle name, and the value is the corresponding declcfg.Bundle object.
	// This map allows quick access to the bundles based on the package and bundle name.
	BundlesByPkgAndName map[string]map[string]declcfg.Bundle
}

type Manifest struct {
	Log clog.PluggableLoggerInterface
}

func New(log clog.PluggableLoggerInterface) ManifestInterface {
	return &Manifest{Log: log}
}

// GetImageIndex - used to get the oci index.json
func (o Manifest) GetImageIndex(dir string) (*v2alpha1.OCISchema, error) {
	var oci *v2alpha1.OCISchema
	indx, err := os.ReadFile(dir + "/" + index)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(indx, &oci)
	if err != nil {
		return nil, err
	}
	return oci, nil
}

// GetImageManifest used to ge the manifest in the oci blobs/sha254
// directory - found in index.json
func (o Manifest) GetImageManifest(file string) (*v2alpha1.OCISchema, error) {
	var oci *v2alpha1.OCISchema
	manifest, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(manifest, &oci)
	if err != nil {
		return nil, err
	}
	return oci, nil
}

// GetOperatorConfig used to parse the operator json
func (o Manifest) GetOperatorConfig(file string) (*v2alpha1.OperatorConfigSchema, error) {
	var ocs *v2alpha1.OperatorConfigSchema
	manifest, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(manifest, &ocs)
	if err != nil {
		return nil, err
	}
	return ocs, nil
}

// ExtractLayersOCI
func (o Manifest) ExtractLayersOCI(fromPath, toPath, label string, oci *v2alpha1.OCISchema) error {
	if _, err := os.Stat(toPath + "/" + label); errors.Is(err, os.ErrNotExist) {
		for _, blob := range oci.Layers {
			validDigest, err := digest.Parse(blob.Digest)
			if err != nil {
				return fmt.Errorf("the digest format is not correct %s ", blob.Digest)
			}
			f, err := os.Open(fromPath + "/" + validDigest.Encoded())
			if err != nil {
				return err
			}
			err = untar(f, toPath, label)
			if err != nil {
				return err
			}
		}
	} else {
		o.Log.Debug("extract directory exists (nop)")
	}
	return nil
}

// GetReleaseSchema
func (o Manifest) GetReleaseSchema(filePath string) ([]v2alpha1.RelatedImage, error) {
	var release = v2alpha1.ReleaseSchema{}

	file, err := os.ReadFile(filePath)
	if err != nil {
		return []v2alpha1.RelatedImage{}, err
	}

	err = json.Unmarshal([]byte(file), &release)
	if err != nil {
		return []v2alpha1.RelatedImage{}, err
	}

	var allImages []v2alpha1.RelatedImage
	for _, item := range release.Spec.Tags {
		allImages = append(allImages, v2alpha1.RelatedImage{Image: item.From.Name, Name: item.Name, Type: v2alpha1.TypeOCPReleaseContent})
	}
	return allImages, nil
}

// UntarLayers simple function that untars the image layers
func untar(gzipStream io.Reader, path string, cfgDirName string) error {
	//Remove any separators in cfgDirName as received from the label
	cfgDirName = strings.TrimSuffix(cfgDirName, "/")
	cfgDirName = strings.TrimPrefix(cfgDirName, "/")
	uncompressedStream, err := gzip.NewReader(gzipStream)
	if err != nil {
		return fmt.Errorf("untar: gzipStream - %w", err)
	}

	tarReader := tar.NewReader(uncompressedStream)
	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf("untar: Next() failed: %s", err.Error())
		}

		if strings.Contains(header.Name, cfgDirName) {
			switch header.Typeflag {
			case tar.TypeDir:
				if header.Name != "./" {
					if err := os.MkdirAll(filepath.Join(path, header.Name), 0755); err != nil {
						return fmt.Errorf("untar: Mkdir() failed: %v", err)
					}
				}
			case tar.TypeReg:
				err := os.MkdirAll(filepath.Dir(filepath.Join(path, header.Name)), 0755)
				if err != nil {
					return fmt.Errorf("untar: Create() failed: %v", err)
				}
				outFile, err := os.Create(filepath.Join(path, header.Name))
				if err != nil {
					return fmt.Errorf("untar: Create() failed: %v", err)
				}
				if _, err := io.Copy(outFile, tarReader); err != nil {
					return fmt.Errorf("untar: Copy() failed: %v", err)
				}
				outFile.Close()

			default:
				// just ignore errors as we are only interested in the FB configs layer
				klog.Warningf("untar: unknown type: %v in %s", header.Typeflag, header.Name)
			}
		}
	}
	return nil
}

func (o Manifest) GetCatalog(filePath string) (OperatorCatalog, error) {
	cfg, err := declcfg.LoadFS(context.Background(), os.DirFS(filePath))

	operatorCatalog := newOperatorCatalog()

	for _, p := range cfg.Packages {
		operatorCatalog.Packages[p.Name] = p
	}

	for _, c := range cfg.Channels {
		operatorCatalog.Channels[c.Package] = append(operatorCatalog.Channels[c.Package], c)
		for _, e := range c.Entries {
			if _, ok := operatorCatalog.ChannelEntries[c.Package]; !ok {
				operatorCatalog.ChannelEntries[c.Package] = make(map[string]map[string]declcfg.ChannelEntry)
			}
			if _, ok := operatorCatalog.ChannelEntries[c.Package][c.Name]; !ok {
				operatorCatalog.ChannelEntries[c.Package][c.Name] = make(map[string]declcfg.ChannelEntry)
			}

			operatorCatalog.ChannelEntries[c.Package][c.Name][e.Name] = e
		}

	}

	for _, b := range cfg.Bundles {
		if _, ok := operatorCatalog.BundlesByPkgAndName[b.Package]; !ok {
			operatorCatalog.BundlesByPkgAndName[b.Package] = make(map[string]declcfg.Bundle)
		}

		if _, ok := operatorCatalog.BundlesByPkgAndName[b.Package][b.Name]; !ok {
			operatorCatalog.BundlesByPkgAndName[b.Package][b.Name] = b
		}
	}

	return operatorCatalog, err
}

func (o Manifest) GetRelatedImagesFromCatalog(operatorCatalog OperatorCatalog, ctlgInIsc v2alpha1.Operator) (map[string][]v2alpha1.RelatedImage, error) {

	relatedImages := make(map[string][]v2alpha1.RelatedImage)

	if len(ctlgInIsc.Packages) == 0 {
		for operatorName := range operatorCatalog.Packages {

			operatorConfig := parseOperatorCatalogByOperator(operatorName, operatorCatalog)

			ri, err := getRelatedImages(o.Log, operatorName, operatorConfig, v2alpha1.IncludePackage{}, ctlgInIsc.Full)

			if err != nil {
				return relatedImages, err
			}

			//TODO GOLANG 1.21 required - replace the for loop below with maps.Copy
			//maps.Copy(relatedImages, ri)

			for k, v := range ri {
				relatedImages[k] = v
			}
		}
	} else {
		for _, iscOperator := range ctlgInIsc.Packages {
			operatorConfig := parseOperatorCatalogByOperator(iscOperator.Name, operatorCatalog)

			ri, err := getRelatedImages(o.Log, iscOperator.Name, operatorConfig, iscOperator, ctlgInIsc.Full)
			if err != nil {
				return relatedImages, err
			}

			//TODO GOLANG 1.21 required - replace the for loop with maps.Copy
			//maps.Copy(relatedImages, ri)

			for k, v := range ri {
				relatedImages[k] = v
			}
			o.Log.Trace("related images %v", relatedImages)
		}
	}

	for k := range relatedImages {
		o.Log.Debug("bundle after filtered : %s", k)
	}

	return relatedImages, nil
}

func newOperatorCatalog() OperatorCatalog {
	operatorConfig := OperatorCatalog{
		Packages:            make(map[string]declcfg.Package),
		Channels:            make(map[string][]declcfg.Channel),
		ChannelEntries:      make(map[string]map[string]map[string]declcfg.ChannelEntry),
		BundlesByPkgAndName: make(map[string]map[string]declcfg.Bundle),
	}

	return operatorConfig
}

func parseOperatorCatalogByOperator(operatorName string, operatorCatalog OperatorCatalog) OperatorCatalog {
	operatorConfig := newOperatorCatalog()
	operatorConfig.Packages[operatorName] = operatorCatalog.Packages[operatorName]
	operatorConfig.Channels[operatorName] = operatorCatalog.Channels[operatorName]
	operatorConfig.ChannelEntries[operatorName] = operatorCatalog.ChannelEntries[operatorName]
	operatorConfig.BundlesByPkgAndName[operatorName] = operatorCatalog.BundlesByPkgAndName[operatorName]

	return operatorConfig
}

func getRelatedImages(log clog.PluggableLoggerInterface, operatorName string, operatorConfig OperatorCatalog, iscOperator v2alpha1.IncludePackage, full bool) (map[string][]v2alpha1.RelatedImage, error) {
	invalid, err := isInvalidFiltering(iscOperator, full)
	if invalid {
		return nil, err
	}

	relatedImages := make(map[string][]v2alpha1.RelatedImage)
	var filteredBundles []string
	defaultChannel := operatorConfig.Packages[operatorName].DefaultChannel

	switch {
	case len(iscOperator.SelectedBundles) > 0:
		for _, iscSelectedBundle := range iscOperator.SelectedBundles {
			bundle, found := operatorConfig.BundlesByPkgAndName[operatorName][iscSelectedBundle.Name]
			if !found {
				log.Warn("bundle %s of operator %s not found in catalog: SKIPPING", iscSelectedBundle.Name, operatorName)
				continue
			}
			relatedImages[bundle.Name] = addTypeToRelatedImages(bundle)
		}
	case len(iscOperator.Channels) > 0:
		for _, iscChannel := range iscOperator.Channels {
			log.Debug("found channel : %v", iscChannel)
			chEntries := operatorConfig.ChannelEntries[operatorName][iscChannel.Name]
			bundles, err := filterBundles(chEntries, iscChannel.IncludeBundle.MinVersion, iscChannel.IncludeBundle.MaxVersion, full)
			if err != nil {
				log.Error(errorSemver, err)
			}
			log.Debug("adding bundles : %s", bundles)
			filteredBundles = append(filteredBundles, bundles...)
		}
	default:
		chEntries := operatorConfig.ChannelEntries[operatorName][defaultChannel]
		bundles, err := filterBundles(chEntries, iscOperator.MinVersion, iscOperator.MaxVersion, full)
		if err != nil {
			log.Error(errorSemver, err)
		}
		log.Debug("adding bundles : %s", bundles)
		filteredBundles = append(filteredBundles, bundles...)
	}

	for _, bundle := range operatorConfig.BundlesByPkgAndName[operatorName] {
		if full {
			if len(filteredBundles) > 0 && len(iscOperator.Channels) > 0 {
				if slices.Contains(filteredBundles, bundle.Name) {
					relatedImages[bundle.Name] = addTypeToRelatedImages(bundle)
				}
			} else {
				relatedImages[bundle.Name] = addTypeToRelatedImages(bundle)
			}
		} else {
			if slices.Contains(filteredBundles, bundle.Name) {
				relatedImages[bundle.Name] = addTypeToRelatedImages(bundle)
			}
		}
	}

	return relatedImages, nil
}

func isInvalidFiltering(pkg v2alpha1.IncludePackage, full bool) (bool, error) {
	invalid := (len(pkg.Channels) > 0 && (pkg.MinVersion != "" || pkg.MaxVersion != "")) ||
		full && (pkg.MinVersion != "" || pkg.MaxVersion != "")
	if invalid {
		return invalid, fmt.Errorf("cannot use channels/full and min/max versions at the same time")
	}
	invalid = len(pkg.SelectedBundles) > 0 && (len(pkg.Channels) > 0 || pkg.MinVersion != "" || pkg.MaxVersion != "")
	if invalid {
		return invalid, fmt.Errorf("cannot use filtering by bundle selection and filtering by channels or min/max versions at the same time")
	}
	invalid = len(pkg.SelectedBundles) > 0 && full
	if invalid {
		return invalid, fmt.Errorf("cannot use filtering by bundle selection and full the same time")
	}
	return false, nil
}

func filterBundles(channelEntries map[string]declcfg.ChannelEntry, min string, max string, full bool) ([]string, error) {
	var minVersion, maxVersion semver.Version
	var err error

	if min != "" {
		minVersion, err = semver.ParseTolerant(min)
		if err != nil {
			return nil, err
		}
	}

	if max != "" {
		maxVersion, err = semver.ParseTolerant(max)
		if err != nil {
			return nil, err
		}
	}

	var filtered []string
	currentHead := semver.MustParse("0.0.0")
	var currentHeadName string
	preReleases := make(map[string]declcfg.ChannelEntry)

	for _, chEntry := range channelEntries {

		version, err := getChannelEntrySemVer(chEntry.Name)
		if err != nil {
			return nil, err
		}

		if isPreRelease(version) {
			pre := make([]string, len(version.Pre))
			for i, pr := range version.Pre {
				pre[i] = pr.String()
			}
			preString := strings.Join(pre, ".")

			preReleases[fmt.Sprintf("%d.%d.%d-%s", version.Major, version.Minor, version.Patch, preString)] = chEntry
		}

		// preReleases that skip the current head of a channel should be considered as head.
		// even if from the semver perspective, they are LT(currentHead)
		if version.GT(currentHead) {
			currentHead = version
			currentHeadName = chEntry.Name
		}

		//Include this bundle to the filtered list if:
		// * its version is prerelease of an already included bundle
		// * its version is between min and max (both defined)
		// * its version is greater than min (defined), and no max is defined (which means up to channel head)
		// * its version is under max (defined) and no min is defined
		if (min == "" || version.GTE(minVersion)) && (max == "" || version.LTE(maxVersion)) {
			// In case full == false and min and max are empty, do not include this bundle:
			// this is the case where there is no filtering, and where only the channel's head shall be included in the output filter.
			if min == "" && max == "" && !full {
				continue
			}
			filtered = append(filtered, chEntry.Name)
		}
	}

	if len(preReleases) > 0 {
		for version, chEntry := range preReleases {
			if isPreReleaseHead(chEntry, currentHeadName) {
				currentHeadName = chEntry.Name

			}

			if isPreReleaseOfFilteredVersion(version, chEntry.Name, filtered) {
				filtered = append(filtered, chEntry.Name)
			}
		}
	}

	if min == "" && max == "" && currentHead.String() != "0.0.0" && !full {
		return []string{currentHeadName}, nil
	}

	return filtered, nil
}

func getChannelEntrySemVer(chEntryName string) (semver.Version, error) {
	nameSplit := strings.Split(chEntryName, ".")
	if len(nameSplit) < 4 {
		return semver.Version{}, fmt.Errorf("incorrect version format %s ", chEntryName)
	}

	version, err := semver.ParseTolerant(strings.Join(nameSplit[1:], "."))
	if err != nil {
		return semver.Version{}, err
	}

	return version, err
}

func isPreRelease(version semver.Version) bool {
	return len(version.Pre) > 0
}

func isPreReleaseHead(channelEntry declcfg.ChannelEntry, currentHead string) bool {
	return slices.Contains(channelEntry.Skips, currentHead) || channelEntry.Replaces == currentHead
}

func isPreReleaseOfFilteredVersion(version string, chEntryName string, filteredVersions []string) bool {
	if slices.Contains(filteredVersions, chEntryName) {
		return false
	}

	for _, filteredVersion := range filteredVersions {
		if strings.Contains(filteredVersion, strings.Split(version, "-")[0]) {
			return true
		}
	}

	return false
}

func addTypeToRelatedImages(bundle declcfg.Bundle) []v2alpha1.RelatedImage {
	var relatedImages []v2alpha1.RelatedImage

	for _, ri := range bundle.RelatedImages {
		relateImage := v2alpha1.RelatedImage{}
		if ri.Image == bundle.Image {
			relateImage.Name = ri.Name
			relateImage.Image = ri.Image
			relateImage.Type = v2alpha1.TypeOperatorBundle
		} else {
			relateImage.Name = ri.Name
			relateImage.Image = ri.Image
			relateImage.Type = v2alpha1.TypeOperatorRelatedImage
		}
		relatedImages = append(relatedImages, relateImage)
	}
	return relatedImages
}

// ConvertIndex converts the index.json to a single manifest which refers to a multi manifest index in the blobs/sha256 directory
// this is necessary because containers/image does not support multi manifest indexes on the top level folder
func (o Manifest) ConvertIndexToSingleManifest(dir string, oci *v2alpha1.OCISchema) error {
	data, err := os.ReadFile(path.Join(dir, "index.json"))
	if err != nil {
		o.Log.Debug(err.Error())
	}
	hash := sha256.Sum256(data)
	digest := hex.EncodeToString(hash[:])
	size := len(data)
	log.Println("Digest:", digest)
	log.Println("Size:", size)

	err = copy.Copy(path.Join(dir, "index.json"), path.Join(dir, "blobs", "sha256", digest))
	if err != nil {
		return err
	}

	idx := v2alpha1.OCISchema{
		SchemaVersion: oci.SchemaVersion,
		Manifests:     []v2alpha1.OCIManifest{{MediaType: oci.MediaType, Digest: "sha256:" + digest, Size: size}},
	}

	idxData, err := json.Marshal(idx)
	if err != nil {
		return err
	}

	// Write the JSON string to a file
	err = os.WriteFile(path.Join(dir, "index.json"), idxData, 0644)
	if err != nil {
		return err
	}

	return nil
}

func (o Manifest) GetDigest(ctx context.Context, sourceCtx *types.SystemContext, imgRef string) (string, error) {
	if err := mirror.ReexecIfNecessaryForImages([]string{imgRef}...); err != nil {
		return "", err
	}

	srcRef, err := alltransports.ParseImageName(imgRef)
	if err != nil {
		return "", fmt.Errorf("invalid source name %s: %v", imgRef, err)
	}

	img, err := srcRef.NewImageSource(ctx, sourceCtx)
	if err != nil {
		return "", err
	}

	manifestBytes, _, err := img.GetManifest(ctx, nil)
	if err != nil {
		return "", err
	}

	digest, err := manifest.Digest(manifestBytes)
	if err != nil {
		return "", err
	}

	var digestString string
	if strings.Contains(digest.String(), ":") {
		digestString = strings.Split(digest.String(), ":")[1]
	}

	return digestString, nil
}
