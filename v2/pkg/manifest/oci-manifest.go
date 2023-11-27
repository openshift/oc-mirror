package manifest

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
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
	GetRelatedImagesFromCatalog(filePath, label string) (map[string][]v1alpha3.RelatedImage, error)
	GetRelatedImagesFromCatalogByFilter(filePath, label string, op v1alpha2.Operator, mp map[string]v1alpha3.ISCPackage) (map[string][]v1alpha3.RelatedImage, error)
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

// operatorImageExtractDir + "/" + label
// GetRelatedImagesFromCatalog
func (o *Manifest) GetRelatedImagesFromCatalog(filePath, label string) (map[string][]v1alpha3.RelatedImage, error) {
	relatedImages := make(map[string][]v1alpha3.RelatedImage)
	files, err := os.ReadDir(filePath)
	if err != nil {
		return relatedImages, err
	}
	for _, file := range files {
		// the catalog.json - does not really conform to json standards
		// this needs some thorough testing
		olm, err := readOperatorCatalog(filePath + "/" + file.Name())
		if err != nil {
			return relatedImages, err
		}
		ri, err := getRelatedImageByDefaultChannel(o.Log, olm)
		if err != nil {
			return relatedImages, err
		}
		// append to relatedImages map
		for k, v := range ri {
			relatedImages[k] = v
		}
	}
	return relatedImages, nil
}

// GetRelatedImagesFromCatalogByFilter
func (o *Manifest) GetRelatedImagesFromCatalogByFilter(filePath, label string, op v1alpha2.Operator, mp map[string]v1alpha3.ISCPackage) (map[string][]v1alpha3.RelatedImage, error) {
	relatedImages := make(map[string][]v1alpha3.RelatedImage)
	for _, pkg := range op.Packages {
		// the catalog.json - does not really conform to json standards
		// this needs some thorough testing
		olm, err := readOperatorCatalog(filePath + "/" + label + "/" + pkg.Name)
		if err != nil {
			return relatedImages, err
		}

		ri, err := getRelatedImageByFilter(o.Log, olm, mp[pkg.Name])
		if err != nil {
			return relatedImages, err
		}
		// append to reletedImages map
		for k, v := range ri {
			relatedImages[k] = v
		}
		o.Log.Trace("related images %v", relatedImages)
	}
	return relatedImages, nil
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
		allImages = append(allImages, v1alpha3.RelatedImage{Image: item.From.Name, Name: item.Name})
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

// readOperatorCatalog - simple function tha treads the specific catalog.json file
// and unmarshals it to DeclarativeConfig struct
func readOperatorCatalog(path string) ([]v1alpha3.DeclarativeConfig, error) {
	// the catalog.json - dos not really conform to json standards
	// this needs some thorough testing
	// operatorImageExtractDir + "/" + label + "/" + name + "/" + catalogJson
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

// getRelatedImageByDefaultChannel - get the DeclarativeConfig for the default channel
// it returns the HEAD (latest version of the bundles relatedImages)
func getRelatedImageByDefaultChannel(log clog.PluggableLoggerInterface, olm []v1alpha3.DeclarativeConfig) (map[string][]v1alpha3.RelatedImage, error) {
	// relevant variables
	relatedImages := make(map[string][]v1alpha3.RelatedImage)
	bundles := make(map[string]bool)
	var defaultChannel string

	// iterate through the catalog objects
	for i, obj := range olm {
		switch {
		case obj.Schema == "olm.channel":
			if defaultChannel == obj.Name {
				log.Debug("found channel : %v", obj)
				log.Debug("bundle image to use : %v", obj.Entries[0].Name)
				name, err := semverFindMax(obj.Entries)
				if err != nil {
					log.Error(errorSemver, err)
				}
				bundles[name] = true
			}
		case obj.Schema == "olm.bundle":
			if bundles[obj.Name] {
				log.Debug("config bundle: %d %v", i, obj.Name)
				log.Trace("config relatedImages: %d %v", i, obj.RelatedImages)
				relatedImages[obj.Name] = obj.RelatedImages
			}
		case obj.Schema == "olm.package":
			log.Debug("Config package: %v", obj.Name)
			defaultChannel = obj.DefaultChannel
		}
	}
	return relatedImages, nil
}

// getRelatedImageByFilter - get the DeclarativeConfig for a specifc channel with
// min,max version if set
func getRelatedImageByFilter(log clog.PluggableLoggerInterface, olm []v1alpha3.DeclarativeConfig, pkg v1alpha3.ISCPackage) (map[string][]v1alpha3.RelatedImage, error) {
	// relevant variables
	relatedImages := make(map[string][]v1alpha3.RelatedImage)
	bundles := make(map[string]bool)
	// iterate through the catalog objects
	for i, obj := range olm {
		switch {
		case obj.Schema == "olm.channel":
			if len(pkg.Channel) > 0 {
				if pkg.Channel == obj.Name {
					log.Debug("found channel : %v", obj)
					name, err := semverFindRange(obj.Entries, pkg.MinVersion, pkg.MaxVersion)
					if err != nil {
						log.Error(errorSemver, err)
					}
					for _, x := range name {
						bundles[x] = true
					}
				}
			} else {
				name, err := semverFindMax(obj.Entries)
				if err != nil {
					log.Error(errorSemver, err)
				}
				log.Debug("adding channel : %s", name)
				bundles[name] = true
			}
		case obj.Schema == "olm.bundle":
			if bundles[obj.Name] && !pkg.Full {
				log.Debug("config bundle: %d %v", i, obj.Name)
				log.Trace("config relatedImages: %d %v", i, obj.RelatedImages)
				relatedImages[obj.Name] = obj.RelatedImages
			}
			// add all bundles
			if pkg.Full {
				relatedImages[obj.Name] = obj.RelatedImages
			}
		case obj.Schema == "olm.package":
			log.Debug("config package: %v", obj.Name)
			bundles[obj.DefaultChannel] = true
		}
	}
	return relatedImages, nil
}

// semverFindMax - finds the max bundle version
func semverFindMax(entries []v1alpha3.ChannelEntry) (string, error) {
	var max semver.Version
	var index int
	for id, s := range entries {
		hld := strings.Split(s.Name, ".")
		// we are only interested in 1,2,3 positions
		if len(hld) < 4 {
			return "", fmt.Errorf("versioning of string is not correct %s ", s.Name)
		}
		hld[1] = strings.Replace(hld[1], "v", "", -1)
		end := strings.Split(hld[3], "-")
		semStr := strings.Join([]string{hld[1], hld[2], end[0]}, ".")
		version, err := semver.Parse(semStr)
		if err != nil {
			return "", err
		}

		if version.Compare(max) == 1 {
			max = version
			index = id
		}
	}
	return entries[index].Name, nil
}

// semverFindRange - finds the bundles between ranges version
func semverFindRange(entries []v1alpha3.ChannelEntry, min, max string) ([]string, error) {

	var minVersion semver.Version
	var maxVersion semver.Version
	var err error
	var results []string

	// parse the min max strings
	if len(min) > 0 {
		minVersion, err = semver.Parse(min)
		if err != nil {
			return []string{}, err
		}
	} else {
		minVersion, _ = semver.Parse("0.0.0")
	}
	if len(max) > 0 {
		maxVersion, err = semver.Parse(max)
		if err != nil {
			return []string{}, err
		}
	} else {
		maxVersion, _ = semver.Parse("9.9.9")
	}

	for _, s := range entries {
		hld := strings.Split(s.Name, ".")
		// we are only interested in 1,2,3 positions
		if len(hld) < 4 {
			return []string{}, fmt.Errorf("versioning of string is not correct %s ", s.Name)
		}
		hld[1] = strings.Replace(hld[1], "v", "", -1)
		end := strings.Split(hld[3], "-")
		semStr := strings.Join([]string{hld[1], hld[2], end[0]}, ".")
		version, err := semver.Parse(semStr)
		if err != nil {
			return []string{}, err
		}
		if version.Compare(maxVersion) <= 0 && version.Compare(minVersion) >= 1 {
			results = append(results, s.Name)
		}
	}
	return results, nil
}
