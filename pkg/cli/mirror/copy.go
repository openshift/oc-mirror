package mirror

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	semver "github.com/blang/semver/v4"
	imagecopy "github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/pkg/sysregistriesv2"

	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

const (
	blobsPath           string = "/blobs/sha256/"
	ociProtocol         string = "oci:"
	configPath          string = "configs/"
	catalogJSON         string = "/catalog.json"
	relatedImages       string = "relatedImages"
	configsLabel        string = "operators.operatorframework.io.index.configs.v1"
	artifactsFolderName string = "olm_artifacts"
)

// RemoteRegFuncs contains the functions to be used for working with remote registries
// In order to be able to mock these external packages,
// we pass them as parameters of bulkImageCopy and bulkImageMirror
type RemoteRegFuncs struct {
	copy           func(ctx context.Context, policyContext *signature.PolicyContext, destRef types.ImageReference, srcRef types.ImageReference, options *imagecopy.Options) (copiedManifest []byte, retErr error)
	mirrorMappings func(cfg v1alpha2.ImageSetConfiguration, images image.TypedImageMapping, insecure bool) error
}

// RegistriesConfig info from config file
type RegistriesConf struct {
	Registry []struct {
		Location           string `toml:"location"`
		Insecure           bool   `toml:"insecure"`
		Blocked            bool   `toml:"blocked"`
		MirrorByDigestOnly bool   `toml:"mirror-by-digest-only"`
		Prefix             string `toml:"prefix"`
		Mirror             []struct {
			Location string `toml:"location"`
			Insecure bool   `toml:"insecure"`
		} `toml:"mirror"`
	} `toml:"registry"`
}

// ReadegistriesConfig reads and parses info from registries.conf file
func ReadRegistriesConfig(file string) (RegistriesConf, error) {
	_, err := os.Stat(file)
	if err != nil {
		return RegistriesConf{}, err
	}

	var regConfig RegistriesConf
	if _, err := toml.DecodeFile(file, &regConfig); err != nil {
		return RegistriesConf{}, err
	}
	return regConfig, nil
}

// getISConfig simple function to read and unmarshal the imagesetconfig
// set via the command line
func (o *MirrorOptions) getISConfig() (*v1alpha2.ImageSetConfiguration, error) {
	var isc *v1alpha2.ImageSetConfiguration
	configData, err := ioutil.ReadFile(o.ConfigPath)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(configData, &isc)
	if err != nil {
		return nil, err
	}
	return isc, nil
}

// •bulkImageCopy•used•to•copy the•relevant•images•(pull•from•a•registry)•to
// •a•local directory↵
func (o *MirrorOptions) bulkImageCopy(isc *v1alpha2.ImageSetConfiguration, srcSkipTLS, dstSkipTLS bool, remoteRegFuncs RemoteRegFuncs) error {

	mapping := image.TypedImageMapping{}

	// artifactsPath is a folder  used to untar the catalog contents, in order to process it and prepare copy.
	// For each operator, a folder with the name of the operator will be created under artifactsPath (ctlcConfigsDir)
	// For the moment, it is created under the working directory, like oc-mirror-workspace
	artifactsPath := artifactsFolderName

	for _, operator := range isc.Mirror.Operators {

		klog.Infof("downloading the catalog image %s\n", operator.Catalog)
		_, _, repo, _, _ := parseImageName(operator.Catalog)
		localOperatorDir := filepath.Join(o.OutputDir, repo)
		if err := os.RemoveAll(localOperatorDir); err != nil {
			klog.Warningf("unable to clear contents of %s: %v", localOperatorDir, err)
		}

		err := copyImage(operator.Catalog, ociProtocol+localOperatorDir, o.SourceSkipTLS, o.DestSkipTLS, o.OCIInsecureSignaturePolicy, remoteRegFuncs)
		if err != nil {
			return fmt.Errorf("copying catalog image %s : %v", operator.Catalog, err)
		}

		// find the layer with the FB config
		catalogContentsDir := filepath.Join(artifactsPath, repo)
		klog.Infof("Finding file based config for %s (in catalog layers)\n", operator.Catalog)
		ctlgConfigsDir, err := o.findFBCConfig(localOperatorDir, catalogContentsDir)
		if err != nil {
			return fmt.Errorf("unable to find config in %s: %v", localOperatorDir, err)
		}

		klog.Infof("Filtering on selected packages for %s \n", operator.Catalog)

		relatedImages, err := getRelatedImages(ctlgConfigsDir, operator.Packages)
		if err != nil {
			return err
		}

		result, err := o.generateSrcToFileMapping(relatedImages)
		if err != nil {
			return err
		}
		mapping.Merge(result)
	}

	o.Dir = strings.TrimPrefix(o.Dir, o.OutputDir+"/")
	if len(mapping) > 0 {
		err := remoteRegFuncs.mirrorMappings(*isc, mapping, srcSkipTLS)
		if err != nil {
			return err
		}
	} else {
		klog.Infof("no images to copy")
	}

	return nil
}

func (o *MirrorOptions) generateSrcToFileMapping(relatedImages []declcfg.RelatedImage) (image.TypedImageMapping, error) {
	mapping := image.TypedImageMapping{}
	for _, i := range relatedImages {
		if i.Image == "" {
			klog.Warningf("invalid related image %s: reference empty", i.Name)
			continue
		}
		name := i.Name
		if name == "" {
			//Creating a unique name for this image, that doesnt have a name
			name = fmt.Sprintf("%x", sha256.Sum256([]byte(i.Image)))[0:6]
		}

		reg, err := sysregistriesv2.FindRegistry(newSystemContext(false), i.Image)
		if err != nil {
			klog.Warningf("Cannot find registry for %s", i.Image)
		}
		if reg != nil && len(reg.Mirrors) > 0 && reg.Mirrors[0].Location != "" {
			mirroredImage := strings.Replace(i.Image, reg.Prefix, reg.Mirrors[0].Location, 1)
			i.Image = mirroredImage
		}

		srcTIR, err := image.ParseReference(i.Image)
		if err != nil {
			return nil, err
		}
		srcTI := image.TypedImage{
			TypedImageReference: srcTIR,
			Category:            v1alpha2.TypeOperatorRelatedImage,
		}
		dstPath := "file://" + name
		if srcTIR.Ref.ID != "" {
			dstPath = dstPath + "/" + strings.TrimPrefix(srcTI.Ref.ID, "sha256:")
		} else if srcTIR.Ref.ID == "" && srcTIR.Ref.Tag != "" {
			//recreating a fake digest to copy image into
			//this is because dclcfg.LoadFS will create a symlink to this folder
			//from the tag
			dstPath = dstPath + "/" + fmt.Sprintf("%x", sha256.Sum256([]byte(srcTIR.Ref.Tag)))[0:6]
		}
		dstTIR, err := image.ParseReference(strings.ToLower(dstPath))
		if err != nil {
			return nil, err
		}
		if srcTI.Ref.Tag != "" {
			//put the tag back because it's needed to follow symlinks by LoadFS
			dstTIR.Ref.Tag = srcTI.Ref.Tag
		}
		dstTI := image.TypedImage{
			TypedImageReference: dstTIR,
			Category:            v1alpha2.TypeOperatorRelatedImage,
		}
		mapping[srcTI] = dstTI
	}
	return mapping, nil
}

// bulkImageMirror used to mirror the relevant images (push from a directory) to
// a remote registry in oci format
func (o *MirrorOptions) bulkImageMirror(isc *v1alpha2.ImageSetConfiguration, destRepo, namespace string, remoteRegFuncs RemoteRegFuncs) error {
	mapping := image.TypedImageMapping{}

	for _, operator := range isc.Mirror.Operators {
		_, _, repo, _, _ := parseImageName(operator.Catalog)
		log.Printf("INFO: processing contents of local catalog %s\n", operator.Catalog)

		// assume that the artifacts are placed in the <current working directory>/olm_artifacts
		artifactsPath := artifactsFolderName

		// This allows for the use case where oc-mirror copy (with the use-oci-feature flag) was not used
		// check to see if the artifactsPath exists
		// if it does it means that oc-mirror copy with the use-oci-feature flag was used and continue normally
		// if it does not exist the function findFBCConfig needs to be called to setup the appropriate directory structures
		// and the directory names need to change

		operatorCatalog := trimProtocol(operator.Catalog)

		// check for the valid config label to use
		configsLabel, err := o.getCatalogConfigPath(operatorCatalog)
		if err != nil {
			log.Fatalf("unable to retrieve configs layer for image %s:\n%v\nMake sure you run oc-mirror with --use-oci-feature and --oci-feature-action=copy prior to executing this step", operator.Catalog, err)
			return err
		}
		// initialize path starting with <current working directory>/olm_artifacts/<repo>
		catalogContentsDir := filepath.Join(artifactsPath, repo)
		// initialize path where we assume the catalog config dir is <current working directory>/olm_artifacts/<repo>/<config folder>
		catalogConfigsDir := filepath.Join(catalogContentsDir, configsLabel)

		if _, err := os.Stat(catalogConfigsDir); os.IsNotExist(err) {
			// get the FBC, and obtain its config directory
			// example: <current working directory>/olm_artifacts/<repo>/<config folder>
			ctlgConfigsDir, err := o.findFBCConfig(operatorCatalog, catalogContentsDir)
			if err != nil {
				return fmt.Errorf("unable to find config in %s: %v", artifactsPath, err)
			}
			// if we get here, we've established that the config folder exists, so use it
			catalogContentsDir = ctlgConfigsDir
			klog.Infof("Catalog configs dir %s\n", catalogContentsDir)
		} else {
			// folder exists, so use as-is
			klog.Infof("Catalog configs dir %s\n", catalogContentsDir)
		}

		klog.Infof("Processing contents of local catalog %s\n", operator.Catalog)

		relatedImages, err := getRelatedImages(catalogContentsDir, operator.Packages)
		if err != nil {
			klog.Fatal(err)
			return err
		}

		// place related images into the workspace
		result, err := o.generateSrcToFileMapping(relatedImages)
		if err != nil {
			return err
		}
		if len(result) > 0 {
			err := remoteRegFuncs.mirrorMappings(*isc, result, o.SourceSkipTLS)
			if err != nil {
				return err
			}
		} else {
			klog.Infof("no images to copy")
		}

		// create mappings for the related images that will moved from the workspace to the final destination
		for _, i := range relatedImages {
			if i.Image == "" {
				klog.Warningf("invalid related image %s: reference empty", i.Name)
				continue
			}
			folder := i.Name
			if folder == "" {
				//Regenerating the unique name for this image, that doesnt have a name
				folder = fmt.Sprintf("%x", sha256.Sum256([]byte(i.Image)))[0:6]
			}
			from, to := "", ""
			_, subns, imgName, tag, sha := parseImageName(i.Image)

			from = folder
			if sha != "" {
				from = from + "/" + strings.TrimPrefix(sha, "sha256:")
			} else if sha == "" && tag != "" {
				from = from + "/" + fmt.Sprintf("%x", sha256.Sum256([]byte(tag)))[0:6]
			}
			if tag != "" {
				to = strings.Join([]string{destRepo, namespace, subns, imgName}, "/") + ":" + tag
			} else {
				to = strings.Join([]string{destRepo, namespace, subns, imgName}, "/") + "@sha256:" + sha
			}
			srcTIR, err := image.ParseReference("file://" + strings.ToLower(from))
			if err != nil {
				return err
			}
			if sha != "" && srcTIR.Ref.ID == "" {
				srcTIR.Ref.ID = "sha256:" + sha
			}
			if tag != "" && srcTIR.Ref.Tag == "" {
				srcTIR.Ref.Tag = tag
			}
			srcTI := image.TypedImage{
				TypedImageReference: srcTIR,
				Category:            v1alpha2.TypeOperatorRelatedImage,
			}

			dstTIR, err := image.ParseReference(to)
			if err != nil {
				return err
			}
			if sha != "" && dstTIR.Ref.ID == "" {
				dstTIR.Ref.ID = "sha256:" + sha
			}
			//If there is no tag mirrorMapping is unable to push the image
			//It would push manifests and layers, but image would not appear
			//in registry
			if sha != "" && dstTIR.Ref.Tag == "" {
				dstTIR.Ref.Tag = sha[0:6]
			}
			dstTI := image.TypedImage{
				TypedImageReference: dstTIR,
				Category:            v1alpha2.TypeOperatorRelatedImage,
			}
			mapping[srcTI] = dstTI
		}

		var to string
		to = strings.Join([]string{"docker://" + destRepo, namespace}, "/")
		if len(o.OCIRegistriesConfig) > 0 {
			regConf, err := ReadRegistriesConfig(o.OCIRegistriesConfig)
			if err != nil {
				klog.Fatalf("reading registries.conf file %v", err)
			} else {
				// this is very basic, it satifies the requirement for WRKLDS-468
				// it's also pointless changing the 'to' destination if there are no relevant entries
				if len(regConf.Registry) > 0 && destRepo == regConf.Registry[0].Location && len(regConf.Registry[0].Mirror) > 0 {
					tmpTo := o.ToMirror
					to = strings.Replace(to, destRepo, regConf.Registry[0].Mirror[0].Location, -1)
					o.ToMirror = regConf.Registry[0].Mirror[0].Location
					o.DestSkipTLS = regConf.Registry[0].Mirror[0].Insecure
					klog.Infof("applied registries.conf mirror location from %s to %s \n", tmpTo, regConf.Registry[0].Mirror[0].Location)
				}
			}
		}

		klog.Infof("pushing catalog %s to %s \n", operator.Catalog, to)

		if operator.TargetName != "" {
			to = strings.Join([]string{to, operator.TargetName}, "/")
		} else {
			to = strings.Join([]string{to, repo}, "/")
		}
		if operator.TargetTag != "" {
			to += ":" + operator.TargetTag
		}
		err = copyImage(operator.Catalog, to, o.DestSkipTLS, o.SourceSkipTLS, o.OCIInsecureSignaturePolicy, remoteRegFuncs)
		if err != nil {
			return err
		}
	}
	err := remoteRegFuncs.mirrorMappings(*isc, mapping, o.DestSkipTLS)
	if err != nil {
		return err
	}

	return nil

}

// findFBCConfig function to find the layer from the catalog
// that has the file based configuration
func (o *MirrorOptions) findFBCConfig(imagePath, catalogContentsPath string) (string, error) {
	// read the index.json of the catalog
	srcImg, err := getOCIImgSrcFromPath(context.TODO(), imagePath)
	if err != nil {
		return "", err
	}
	manifest, err := getManifest(context.TODO(), srcImg)
	if err != nil {
		return "", err
	}

	//Use the label in the config layer to determine the
	//folder containing the related images, when untarring layers
	cfgDirName, err := getConfigPathFromConfigLayer(imagePath, string(manifest.ConfigInfo().Digest))
	if err != nil {
		return "", err
	}
	// iterate through each layer

	for _, layer := range manifest.LayerInfos() {
		layerSha := layer.Digest.String()
		layerDirName := layerSha[7:]
		r, err := os.Open(imagePath + blobsPath + layerDirName)
		if err != nil {
			return "", err
		}
		// untar if it is the FBC
		err = UntarLayers(r, catalogContentsPath, cfgDirName)
		if err != nil {
			return "", err
		}
	}
	cfgContentsPath := filepath.Join(catalogContentsPath, cfgDirName)
	f, err := os.Open(cfgContentsPath)
	if err != nil {
		return "", fmt.Errorf("unable to open temp folder containing extracted catalogs %s: %w", cfgContentsPath, err)
	}
	contents, err := f.Readdir(0)
	if err != nil {
		return "", fmt.Errorf("unable to read temp folder containing extracted catalogs %s: %w", cfgContentsPath, err)
	}
	if len(contents) == 0 {
		return "", fmt.Errorf("no packages found in catalog")
	}
	return cfgContentsPath, nil
}

// getCatalogConfigPath takes an OCI FBC image as an input,
// it reads the manifest, then the config layer,
// more specifically the label `configLabel`
// and returns the value of that label
// The function fails if more than one manifest exist in the image
func (o *MirrorOptions) getCatalogConfigPath(imagePath string) (string, error) {
	// read the index.json of the catalog
	srcImg, err := getOCIImgSrcFromPath(context.TODO(), imagePath)
	if err != nil {
		return "", err
	}
	manifest, err := getManifest(context.TODO(), srcImg)
	if err != nil {
		return "", err
	}

	//Use the label in the config layer to determine the
	//folder containing the related images, when untarring layers
	cfgDirName, err := getConfigPathFromConfigLayer(imagePath, string(manifest.ConfigInfo().Digest))
	if err != nil {
		return "", err
	}
	return cfgDirName, nil
}

func getConfigPathFromConfigLayer(imagePath, configSha string) (string, error) {
	var cfg *manifest.Schema2V1Image
	configLayerDir := configSha[7:]
	cfgBlob, err := ioutil.ReadFile(filepath.Join(imagePath, blobsPath, configLayerDir))
	if err != nil {
		return "", fmt.Errorf("unable to read the config blob %s from the oci image: %w", configLayerDir, err)
	}
	err = json.Unmarshal(cfgBlob, &cfg)
	if err != nil {
		return "", fmt.Errorf("problem unmarshaling config blob in %s: %w", configLayerDir, err)
	}
	if dirName, ok := cfg.Config.Labels[configsLabel]; ok {
		return dirName, nil
	}
	return "", fmt.Errorf("label %s not found in config blob %s", configsLabel, configLayerDir)
}

// getRelatedImages reads a directory containing an FBC catalog () unpacked contents
// and returns the list of relatedImages found in the CSVs of bundles
// filtering by the list of packages provided in imageSetConfig for the catalog
func getRelatedImages(directory string, packages []v1alpha2.IncludePackage) ([]declcfg.RelatedImage, error) {
	allImages := []declcfg.RelatedImage{}
	// load the declarative config from the provided directory (if possible)
	cfg, err := declcfg.LoadFS(os.DirFS(directory))
	if err != nil {
		return nil, err
	}

	if len(packages) == 0 {
		for _, aPackage := range cfg.Packages {
			packages = append(packages, v1alpha2.IncludePackage{
				Name: aPackage.Name,
			})
		}
	}

	for _, bundle := range cfg.Bundles {
		isSelected, err := isPackageSelected(bundle, cfg.Channels, packages)
		if err != nil {
			return nil, err
		}
		if isSelected {
			allImages = append(allImages, declcfg.RelatedImage{Name: bundle.Package, Image: bundle.Image})
			allImages = append(allImages, bundle.RelatedImages...)
		}
	}
	//make sure there are no duplicates in the list with same image:
	finalList := []declcfg.RelatedImage{}
	for _, i := range allImages {
		found := false
		for _, j := range finalList {
			if i.Image == j.Image {
				found = true
				break
			}
		}
		if !found {
			finalList = append(finalList, i)
		}
	}
	return finalList, nil
}

func isPackageSelected(bundle declcfg.Bundle, channels []declcfg.Channel, packages []v1alpha2.IncludePackage) (bool, error) {
	isSelected := false
	for _, pkg := range packages {
		if pkg.Name == bundle.Package {
			var min, max semver.Version
			if pkg.MinVersion != "" || pkg.MaxVersion != "" {
				version_string, err := bundleVersion(bundle)
				if err != nil {
					return isSelected, err
				}
				pkgVer, err := semver.Make(version_string)
				if err != nil {
					return isSelected, err
				}
				if err != nil {
					return isSelected, err
				}
				if pkg.MinVersion != "" {
					min, err = semver.Make(pkg.MinVersion)
					if err != nil {
						return isSelected, err
					}
				}
				if pkg.MaxVersion != "" {
					max, err = semver.Make(pkg.MaxVersion)
					if err != nil {
						return isSelected, err
					}
				}

				if (pkg.MinVersion != "" && pkg.MaxVersion != "") && pkgVer.Compare(min) >= 0 && pkgVer.Compare(max) <= 0 {
					isSelected = true
				} else if pkg.MinVersion != "" && pkg.MaxVersion == "" && pkgVer.Compare(min) >= 0 {
					isSelected = true
				} else if pkg.MaxVersion != "" && pkg.MinVersion == "" && pkgVer.Compare(max) <= 0 {
					isSelected = true
				}

			} else { // no filtering required
				isSelected = true
			}
		}
	}
	return isSelected, nil
}

func bundleVersion(bundle declcfg.Bundle) (string, error) {
	for _, prop := range bundle.Properties {
		if prop.Type == property.TypePackage {
			var p property.Package
			if err := json.Unmarshal(prop.Value, &p); err != nil {
				return "", err
			}
			return p.Version, nil
		}
	}
	return "", fmt.Errorf("unable to find bundle version")
}

// getManifest reads the manifest of the OCI FBC image
// and returns it as a go structure of type manifest.Manifest
func getManifest(ctx context.Context, imgSrc types.ImageSource) (manifest.Manifest, error) {
	manifestBlob, manifestType, err := imgSrc.GetManifest(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to get manifest blob from image : %w", err)
	}
	manifest, err := manifest.FromBlob(manifestBlob, manifestType)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshall manifest of image : %w", err)
	}
	return manifest, nil
}

// getOCIImgSrcFromPath tries to "load" the OCI FBC image in the path
// for further processing.
// It supports path strings with or without the protocol (oci:) prefix
func getOCIImgSrcFromPath(ctx context.Context, path string) (types.ImageSource, error) {
	if !strings.HasPrefix(path, "oci") {
		path = ociProtocol + path
	}
	ociImgRef, err := alltransports.ParseImageName(path)
	if err != nil {
		return nil, err
	}
	imgsrc, err := ociImgRef.NewImageSource(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to get OCI Image from %s: %w", path, err)
	}
	return imgsrc, nil
}

// parseImageName returns the registry, organisation, repository, tag and digest
// from the imageName.
// It can handle both remote and local images.
func parseImageName(imageName string) (string, string, string, string, string) {
	registry, org, repo, tag, sha := "", "", "", "", ""
	imageName = trimProtocol(imageName)
	imageName = strings.TrimPrefix(imageName, "/")
	imageName = strings.TrimSuffix(imageName, "/")
	tmp := strings.Split(imageName, "/")

	registry = tmp[0]
	img := strings.Split(tmp[len(tmp)-1], ":")
	if len(tmp) > 2 {
		org = strings.Join(tmp[1:len(tmp)-1], "/")
	}
	if len(img) > 1 {
		if strings.Contains(img[0], "@") {
			nm := strings.Split(img[0], "@")
			repo = nm[0]
			sha = img[1]
		} else {
			repo = img[0]
			tag = img[1]
		}
	} else {
		repo = img[0]
	}

	return registry, org, repo, tag, sha
}

// trimProtocol removes oci://, file:// or docker:// from
// the parameter imageName
func trimProtocol(imageName string) string {
	imageName = strings.TrimPrefix(imageName, "oci:")
	imageName = strings.TrimPrefix(imageName, "file:")
	imageName = strings.TrimPrefix(imageName, "docker:")
	imageName = strings.TrimPrefix(imageName, "//")

	return imageName
}

// UntarLayers simple function that untars the layer that
// has the FB configuration
func UntarLayers(gzipStream io.Reader, path string, cfgDirName string) error {
	//Remove any separators in cfgDirName as received from the label
	cfgDirName = strings.TrimSuffix(cfgDirName, "/")
	cfgDirName = strings.TrimPrefix(cfgDirName, "/")
	uncompressedStream, err := gzip.NewReader(gzipStream)
	if err != nil {
		return fmt.Errorf("UntarLayers: NewReader failed - %w", err)
	}

	tarReader := tar.NewReader(uncompressedStream)
	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf("UntarLayers: Next() failed: %s", err.Error())
		}

		if strings.Contains(header.Name, cfgDirName) {
			switch header.Typeflag {
			case tar.TypeDir:
				if header.Name != "./" {
					if err := os.MkdirAll(path+"/"+header.Name, 0755); err != nil {
						return fmt.Errorf("UntarLayers: Mkdir() failed: %v", err)
					}
				}
			case tar.TypeReg:
				outFile, err := os.Create(path + "/" + header.Name)
				if err != nil {
					return fmt.Errorf("UntarLayers: Create() failed: %v", err)
				}
				if _, err := io.Copy(outFile, tarReader); err != nil {
					return fmt.Errorf("UntarLayers: Copy() failed: %v", err)
				}
				outFile.Close()

			default:
				// just ignore errors as we are only interested in the FB configs layer
				klog.Warningf("UntarLayers: unknown type: %v in %s", header.Typeflag, header.Name)
			}
		}
	}
	return nil
}

// copyImage is here used only to push images to the remote registry
// calls the underlying containers/image copy library
func copyImage(from, to string, srcSkipTLS bool, dstSkipTLS bool, insecurePolicy bool, funcs RemoteRegFuncs) error {
	if !strings.HasPrefix(from, "docker") {
		// find absolute path if from is a relative path
		fromPath := trimProtocol(from)
		if !strings.HasPrefix(fromPath, "/") {
			absolutePath, err := filepath.Abs(fromPath)
			if err != nil {
				return fmt.Errorf("unable to get absolute path of oci image %s: %v", from, err)
			}
			from = "oci://" + absolutePath
		}
	}

	sourceCtx := newSystemContext(srcSkipTLS)
	destinationCtx := newSystemContext(dstSkipTLS)
	ctx := context.Background()

	// Pull the source image, and store it in the local storage, under the name main
	var sigPolicy *signature.Policy
	var err error
	if insecurePolicy {
		sigPolicy = &signature.Policy{Default: []signature.PolicyRequirement{signature.NewPRInsecureAcceptAnything()}}
	} else {
		sigPolicy, err = signature.DefaultPolicy(nil)
		if err != nil {
			return err
		}
	}
	policyContext, err := signature.NewPolicyContext(sigPolicy)
	if err != nil {
		return err
	}
	// define the source context
	srcRef, err := alltransports.ParseImageName(from)
	if err != nil {
		return err
	}
	// define the destination context
	destRef, err := alltransports.ParseImageName(to)
	if err != nil {
		return err
	}

	// call the copy.Image function with the set options
	_, err = funcs.copy(ctx, policyContext, destRef, srcRef, &imagecopy.Options{
		RemoveSignatures:      true,
		SignBy:                "",
		ReportWriter:          os.Stdout,
		SourceCtx:             sourceCtx,
		DestinationCtx:        destinationCtx,
		ForceManifestMIMEType: "",
		ImageListSelection:    imagecopy.CopySystemImage,
		OciDecryptConfig:      nil,
		OciEncryptLayers:      nil,
		OciEncryptConfig:      nil,
	})
	if err != nil {
		return err
	}
	return nil
}

// newSystemContext set the context for source & destination resources
func newSystemContext(skipTLS bool) *types.SystemContext {
	skipTLSVerify := types.OptionalBoolFalse
	if skipTLS {
		skipTLSVerify = types.OptionalBoolTrue
	}
	ctx := &types.SystemContext{
		RegistriesDirPath:           "",
		ArchitectureChoice:          "",
		OSChoice:                    "",
		VariantChoice:               "",
		BigFilesTemporaryDir:        "", //*globalArgs.cache + "/tmp",
		DockerInsecureSkipTLSVerify: skipTLSVerify,
		SystemRegistriesConfPath:    "/home/skhoury/go/src/github.com/openshift/oc-mirror/registries.conf",
	}
	return ctx
}
