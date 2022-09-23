package mirror

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/operator-framework/operator-registry/alpha/model"

	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"sigs.k8s.io/yaml"
)

const (
	blobsPath      string = "/blobs/sha256/"
	tempPath       string = "tmp/"
	indexJSON      string = "/index.json"
	dockerProtocol string = "docker://"
	ociProtocol    string = "oci:"
	configPath     string = "configs/"
	catalogJSON    string = "/catalog.json"
	relatedImages  string = "relatedImages"
	configsLabel   string = "operators.operatorframework.io.index.configs.v1"
)

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
				fmt.Println(fmt.Printf("UntarLayers: unknown type: %v in %s", header.Typeflag, header.Name))
			}
		}
	}
	return nil
}

// newSystemContext set the context for source & destination resources
func newSystemContext(skipTLS bool) *types.SystemContext {
	var skipTLSVerify types.OptionalBool
	if skipTLS {
		skipTLSVerify = types.OptionalBoolTrue
	} else {
		skipTLSVerify = types.OptionalBoolFalse
	}
	ctx := &types.SystemContext{
		RegistriesDirPath:           "",
		ArchitectureChoice:          "",
		OSChoice:                    "",
		VariantChoice:               "",
		BigFilesTemporaryDir:        "", //*globalArgs.cache + "/tmp",
		DockerInsecureSkipTLSVerify: skipTLSVerify,
	}
	return ctx
}

// getISConfig - simple function to read and unmarshal the imagesetconfig
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

//•bulkImageCopy•used•to•copy the•relevant•images•(pull•from•a•registry)•to
//•a•local directory in oci format↵
func bulkImageCopy(isc *v1alpha2.ImageSetConfiguration, srcSkipTLS, dstSkipTLS bool) error {

	files, err := ioutil.ReadDir(tempPath + configPath)
	if err != nil {
		log.Fatal(err)
		return err
	}

	ch := make(chan byte, 1)
	for _, pkg := range isc.Mirror.Operators[0].Packages {
		for _, file := range files {
			if strings.Contains(pkg.Name, file.Name()) {
				fmt.Println(file.Name())
				// read the config.json to get releated images
				relatedImages, err := getRelatedImages(tempPath + configPath + file.Name())
				if err != nil {
					return err
				}
				for _, i := range relatedImages {
					go func() {
						name := i.Name
						if name == "" {
							name = "bundle"
						}
						err := copyImage(dockerProtocol+i.Image, ociProtocol+tempPath+configPath+file.Name()+"/"+name, srcSkipTLS, dstSkipTLS)
						if err != nil {
							log.Fatal(err)
						}
						ch <- 1
					}()
					<-ch
				}
			}
		}
	}
	return nil
}

// bulkImageMirror used to mirror the relevant images (push from a directory) to
// a remote registry in oci format
func bulkImageMirror(isc *v1alpha2.ImageSetConfiguration, imgdest, namespace string, srcSkipTLS, dstSkipTLS bool) error {

	ch := make(chan byte, 1)
	for _, pkg := range isc.Mirror.Operators[0].Packages {
		relatedImages, err := getRelatedImages(tempPath + configPath + pkg.Name)
		if err != nil {
			log.Fatal(err)
			return err
		}

		for _, i := range relatedImages {
			go func() {
				folder := i.Name
				if folder == "" {
					folder = "bundle"
				}
				to, subns, imgName, tag := "", "", "", ""
				tmp := strings.Split(i.Image, "/")
				fmt.Println("DEBUG LMZ ", tmp)
				img := strings.Split(tmp[len(tmp)-1], ":")
				if len(tmp) > 2 {
					subns = strings.Join(tmp[1:len(tmp)-1], "/")
				}
				if strings.Contains(img[0], "@") {
					nm := strings.Split(img[0], "@")
					imgName = nm[0]
					//sha = img[1]
				} else {
					imgName = img[0]
					tag = img[1]
				}

				from := ociProtocol + tempPath + configPath + pkg.Name + "/" + folder
				if tag != "" {
					to = dockerProtocol + strings.Join([]string{imgdest, namespace, subns, imgName}, "/") + ":" + tag
				} else {
					to = dockerProtocol + strings.Join([]string{imgdest, namespace, subns, imgName}, "/") // + "@sha256:" + sha
				}
				fmt.Println("copyImage(" + from + "," + to)
				err := copyImage(from, to, srcSkipTLS, dstSkipTLS)
				if err != nil {
					log.Fatal(err)
				}
				ch <- 1
			}()
			<-ch
		}
	}
	return nil

}

// copyImage function that sets the correct context and
// calls the undrlying container copy library
func copyImage(from, to string, srcSkipTLS bool, dstSkipTLS bool) error {

	sourceCtx := newSystemContext(srcSkipTLS)
	destinationCtx := newSystemContext(dstSkipTLS)
	ctx := context.Background()

	// Pull the source image, and store it in the local storage, under the name main
	policy, err := signature.DefaultPolicy(nil)
	if err != nil {
		return err
	}
	policyContext, err := signature.NewPolicyContext(policy)
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
	_, err = copy.Image(ctx, policyContext, destRef, srcRef,
		&copy.Options{
			RemoveSignatures:      true,
			SignBy:                "",
			ReportWriter:          os.Stdout,
			SourceCtx:             sourceCtx,
			DestinationCtx:        destinationCtx,
			ForceManifestMIMEType: "",
			ImageListSelection:    copy.CopySystemImage,
			OciDecryptConfig:      nil,
			OciEncryptLayers:      nil,
			OciEncryptConfig:      nil,
		})
	if err != nil {
		return err
	}
	return nil
}

// FindFBCConfig function to find the layer from the catalog
// that has the file based configuration
func (o *MirrorOptions) FindFBCConfig(path string) error {
	// read the index.json of the catalog
	srcImg, err := getOCIImgSrcFromPath(context.TODO(), path)
	if err != nil {
		return err
	}
	manifest, err := getManifest(context.TODO(), srcImg)
	if err != nil {
		return err
	}

	//Use the label in the config layer to determine the
	//folder containing the related images, when untarring layers
	cfgDirName, err := getConfigPathFromLabel(path, string(manifest.ConfigInfo().Digest))
	if err != nil {
		return err
	}
	// iterate through each layer

	for _, layer := range manifest.LayerInfos() {
		layerSha := layer.Digest.String()
		layerDirName := layerSha[7:]
		fmt.Println(path + blobsPath + layerDirName)
		r, err := os.Open(path + blobsPath + layerDirName)
		if err != nil {
			return err
		}
		// untar if it is the FBC
		err = UntarLayers(r, tempPath, cfgDirName)
		if err != nil {
			return err
		}
	}
	f, err := os.Open(filepath.Join(tempPath, cfgDirName))
	if err != nil {
		return fmt.Errorf("unable to open temp folder containing extracted catalogs %s: %w", filepath.Join(tempPath, cfgDirName), err)
	}
	contents, err := f.Readdir(0)
	if err != nil {
		return fmt.Errorf("unable to read temp folder containing extracted catalogs %s: %w", filepath.Join(tempPath, cfgDirName), err)
	}
	if len(contents) == 0 {
		return fmt.Errorf("no packages found in catalog")
	}
	return nil
}

// getRelatedImages this reads each catalog or config.json
// file in a given operator in the FBC
func getRelatedImages(path string) ([]model.RelatedImage, error) {
	var icJSON *model.Bundle

	// read the config.json to get releated images
	icd, err := ioutil.ReadFile(path + catalogJSON)
	if err != nil {
		return nil, err
	}
	// we are only interested in the related images section
	tmp := string(icd)
	i := strings.Index(tmp, relatedImages)
	newJson := "{\"" + tmp[i:]
	err = json.Unmarshal([]byte(newJson), &icJSON)
	if err != nil {
		return nil, fmt.Errorf("json unmarshal %v", err)
	}
	return icJSON.RelatedImages, nil
}

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

func getOCIImgSrcFromPath(ctx context.Context, path string) (types.ImageSource, error) {
	ociImgRef, err := alltransports.ParseImageName(ociProtocol + path)
	if err != nil {
		return nil, err
	}
	imgsrc, err := ociImgRef.NewImageSource(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to get OCI Image from %s: %w", path, err)
	}
	return imgsrc, nil
}

func getConfigPathFromLabel(imagePath, configSha string) (string, error) {
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
