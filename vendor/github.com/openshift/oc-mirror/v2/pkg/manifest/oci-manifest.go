package manifest

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	digest "github.com/opencontainers/go-digest"

	"github.com/blang/semver/v4"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"k8s.io/klog/v2"
)

const (
	index                   string = "index.json"
	catalogJson             string = "catalog.json"
	operatorImageExtractDir string = "hold-operator"
	errorSemver             string = " semver %v "
)

type ManifestInterface interface {
	GetImageIndex(dir string) (*v1alpha3.OCISchema, error)
	GetImageManifest(file string) (*v1alpha3.OCISchema, error)
	GetOperatorConfig(file string) (*v1alpha3.OperatorConfigSchema, error)
	GetRelatedImagesFromCatalog(filePath, label string, ctlgInIsc v1alpha2.Operator) (map[string][]v1alpha3.RelatedImage, error)
	GetRelatedImagesFromCatalogByFilter(filePath, label string, ctlgInIsc v1alpha2.Operator) (map[string][]v1alpha3.RelatedImage, error)
	ExtractLayersOCI(filePath, toPath, label string, oci *v1alpha3.OCISchema) error
	GetReleaseSchema(filePath string) ([]v1alpha3.RelatedImage, error)
}

type Manifest struct {
	Log clog.PluggableLoggerInterface
}

func New(log clog.PluggableLoggerInterface) ManifestInterface {
	return &Manifest{Log: log}
}

// GetImageIndex - used to get the oci index.json
func (o *Manifest) GetImageIndex(dir string) (*v1alpha3.OCISchema, error) {
	var oci *v1alpha3.OCISchema
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
func (o *Manifest) GetImageManifest(file string) (*v1alpha3.OCISchema, error) {
	var oci *v1alpha3.OCISchema
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
func (o *Manifest) GetOperatorConfig(file string) (*v1alpha3.OperatorConfigSchema, error) {
	var ocs *v1alpha3.OperatorConfigSchema
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
func (o *Manifest) ExtractLayersOCI(fromPath, toPath, label string, oci *v1alpha3.OCISchema) error {
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
		o.Log.Info("extract directory exists (nop)")
	}
	return nil
}

// GetReleaseSchema
func (o *Manifest) GetReleaseSchema(filePath string) ([]v1alpha3.RelatedImage, error) {
	var release = v1alpha3.ReleaseSchema{}

	file, err := os.ReadFile(filePath)
	if err != nil {
		return []v1alpha3.RelatedImage{}, err
	}

	err = json.Unmarshal([]byte(file), &release)
	if err != nil {
		return []v1alpha3.RelatedImage{}, err
	}

	var allImages []v1alpha3.RelatedImage
	for _, item := range release.Spec.Tags {
		allImages = append(allImages, v1alpha3.RelatedImage{Image: item.From.Name, Name: item.Name, Type: v1alpha2.TypeOCPReleaseContent})
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
					if err := os.MkdirAll(path+"/"+header.Name, 0755); err != nil {
						return fmt.Errorf("untar: Mkdir() failed: %v", err)
					}
				}
			case tar.TypeReg:
				outFile, err := os.Create(path + "/" + header.Name)
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

// operatorImageExtractDir + "/" + label
// GetRelatedImagesFromCatalog
func (o *Manifest) GetRelatedImagesFromCatalog(filePath, label string, ctlgInIsc v1alpha2.Operator) (map[string][]v1alpha3.RelatedImage, error) {
	relatedImages := make(map[string][]v1alpha3.RelatedImage)
	operators, err := os.ReadDir(filePath + label)
	if err != nil {
		return relatedImages, err
	}
	for _, operator := range operators {
		// the catalog.json - does not really conform to json standards
		// this needs some thorough testing
		operatorConfig, err := readOperatorConfig(filepath.Join(filePath, label, operator.Name()))
		if err != nil {
			return relatedImages, err
		}
		ri, err := getRelatedImages(o.Log, operatorConfig, v1alpha2.IncludePackage{}, ctlgInIsc.Full)
		if err != nil {
			return relatedImages, err
		}

		for k, v := range ri {
			relatedImages[k] = v
		}
	}
	return relatedImages, nil
}

// GetRelatedImagesFromCatalogByFilter
func (o *Manifest) GetRelatedImagesFromCatalogByFilter(filePath, label string, ctlgInIsc v1alpha2.Operator) (map[string][]v1alpha3.RelatedImage, error) {
	relatedImages := make(map[string][]v1alpha3.RelatedImage)
	for _, iscOperator := range ctlgInIsc.Packages {
		operatorConfig, err := readOperatorConfig(filepath.Join(filePath, label, iscOperator.Name))
		if err != nil {
			return relatedImages, err
		}

		ri, err := getRelatedImages(o.Log, operatorConfig, iscOperator, ctlgInIsc.Full)
		if err != nil {
			return relatedImages, err
		}

		for k, v := range ri {
			relatedImages[k] = v
		}
		o.Log.Trace("related images %v", relatedImages)
	}
	return relatedImages, nil
}

// readOperatorConfig - reads the catalog.json and unmarshals it to DeclarativeConfig struct
func readOperatorConfig(path string) ([]v1alpha3.DeclarativeConfig, error) {
	var olm []v1alpha3.DeclarativeConfig
	data, err := os.ReadFile(path + "/" + catalogJson)
	if err != nil {
		return []v1alpha3.DeclarativeConfig{}, err
	}
	tmp := strings.NewReplacer(" ", "").Replace(string(data))
	updatedJson := "[" + strings.ReplaceAll(tmp, "}\n{", "},{") + "]"
	err = json.Unmarshal([]byte(updatedJson), &olm)
	if err != nil {
		return []v1alpha3.DeclarativeConfig{}, err
	}
	return olm, nil
}

// getRelatedImages - get the packages' related images of a catalog.
func getRelatedImages(log clog.PluggableLoggerInterface, operatorConfig []v1alpha3.DeclarativeConfig, iscOperator v1alpha2.IncludePackage, full bool) (map[string][]v1alpha3.RelatedImage, error) {
	relatedImages := make(map[string][]v1alpha3.RelatedImage)
	bundles := make(map[string]bool)
	var defaultChannel string

	if isInvalidFiltering(iscOperator, full) {
		return nil, fmt.Errorf("cannot use channels/full and min/max versions at the same time")
	}

	for i, obj := range operatorConfig {
		switch {
		case obj.Schema == "olm.channel":
			if len(iscOperator.Channels) > 0 {
				if found, idx := containsChannel(iscOperator.Channels, obj.Name); found {
					log.Debug("found channel : %v", obj)
					filteredBundles, err := filterBundles(obj.Entries, iscOperator.Channels[idx].IncludeBundle.MinVersion, iscOperator.Channels[idx].IncludeBundle.MaxVersion, full)
					if err != nil {
						log.Error(errorSemver, err)
					}
					for _, b := range filteredBundles {
						bundles[b] = true
					}
				}
			} else {
				if defaultChannel == obj.Name {
					filteredBundles, err := filterBundles(obj.Entries, iscOperator.MinVersion, iscOperator.MaxVersion, full)
					if err != nil {
						log.Error(errorSemver, err)
					}
					log.Debug("adding bundles : %s", filteredBundles)
					for _, b := range filteredBundles {
						bundles[b] = true
					}
				}
			}
		case obj.Schema == "olm.bundle":
			if bundles[obj.Name] && !full {
				log.Debug("config bundle: %d %v", i, obj.Name)
				log.Trace("config relatedImages: %d %v", i, obj.RelatedImages)
				relatedImages[obj.Name] = addTypeToRelatedImages(obj)
			}
			// add all bundles
			if full {
				if len(bundles) > 0 && len(iscOperator.Channels) > 0 {
					if bundles[obj.Name] {
						relatedImages[obj.Name] = addTypeToRelatedImages(obj)
					}
				} else {
					relatedImages[obj.Name] = addTypeToRelatedImages(obj)
				}
			}
		case obj.Schema == "olm.package":
			log.Debug("config package: %v", obj.Name)
			bundles[obj.DefaultChannel] = true
			defaultChannel = obj.DefaultChannel
		}
	}
	return relatedImages, nil
}

func isInvalidFiltering(pkg v1alpha2.IncludePackage, full bool) bool {
	return (len(pkg.Channels) > 0 && (pkg.MinVersion != "" || pkg.MaxVersion != "")) ||
		full && (pkg.MinVersion != "" || pkg.MaxVersion != "")
}

func containsChannel(channels []v1alpha2.IncludeChannel, name string) (bool, int) {
	for idx, channel := range channels {
		if channel.Name == name {
			return true, idx
		}
	}
	return false, -1
}

func filterBundles(channelEntries []v1alpha3.ChannelEntry, min string, max string, full bool) ([]string, error) {
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

	for _, chEntry := range channelEntries {

		version, err := getChannelEntrySemVer(chEntry.Name)
		if err != nil {
			return nil, err
		}

		// preReleases that skip the current head of a channel should be considered as head.
		// even if from the semver perspective, they are LT(currentHead)
		if version.GT(currentHead) || isPreReleaseHead(version, chEntry, currentHeadName) {
			currentHead = version
			currentHeadName = chEntry.Name
		}

		//Include this bundle to the filtered list if:
		// * its version is prerelease of an already included bundle
		// * its version is between min and max (both defined)
		// * its version is greater than min (defined), and no max is defined (which means up to channel head)
		// * its version is under max (defined) and no min is defined
		if ((min == "" || version.GTE(minVersion)) && (max == "" || version.LTE(maxVersion))) || isPreReleaseOfFilteredVersion(version, filtered) {
			// In case full == false and min and max are empty, do not include this bundle:
			// this is the case where there is no filtering, and where only the channel's head shall be included in the output filter.
			if min == "" && max == "" && !full {
				continue
			}
			filtered = append(filtered, chEntry.Name)
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
	end := nameSplit[3]
	if strings.Contains(nameSplit[3], "-") {
		end = strings.Join(nameSplit[3:], ".")
	}
	semStr := strings.Join([]string{nameSplit[1], nameSplit[2], end}, ".")

	version, err := semver.ParseTolerant(semStr)
	if err != nil {
		return semver.Version{}, err
	}

	return version, err
}

func isPreReleaseHead(version semver.Version, channelEntry v1alpha3.ChannelEntry, currentHead string) bool {
	return len(version.Pre) > 0 && slices.Contains(channelEntry.Skips, currentHead)
}

func isPreReleaseOfFilteredVersion(semver semver.Version, filteredVersions []string) bool {
	if len(semver.Pre) > 0 {
		for _, filteredVersion := range filteredVersions {
			if strings.Contains(filteredVersion, fmt.Sprintf("%d.%d.%d", semver.Major, semver.Minor, semver.Patch)) {
				return true
			}
		}
	}

	return false
}

func addTypeToRelatedImages(dlcg v1alpha3.DeclarativeConfig) []v1alpha3.RelatedImage {
	var relatedImages []v1alpha3.RelatedImage

	for _, ri := range dlcg.RelatedImages {
		if ri.Image == dlcg.Image {
			ri.Type = v1alpha2.TypeOperatorBundle
		} else {
			ri.Type = v1alpha2.TypeOperatorRelatedImage
		}
		relatedImages = append(relatedImages, ri)
	}
	return relatedImages
}
