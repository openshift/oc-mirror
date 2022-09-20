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
	"strings"

	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
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
)

var globalArgs struct {
	root               *string
	cache              *string
	registriesConfPath *string
}

// Index struct used to decode index.json
type Index struct {
	SchemaVersion int `json:"schemaVersion"`
	Manifests     []struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int    `json:"size"`
	} `json:"manifests"`
}

// OCIImageManifest - oci image manifest
type OCIImageManifest struct {
	SchemaVersion int `json:"schemaVersion"`
	Config        struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int    `json:"size"`
	} `json:"config"`
	Layers      []Layer `json:"layers"`
	Annotations struct {
		OrgOpencontainersImageBaseDigest string `json:"org.opencontainers.image.base.digest"`
		OrgOpencontainersImageBaseName   string `json:"org.opencontainers.image.base.name"`
	} `json:"annotations"`
}

// Layer schema used in OCIImageManifest
type Layer struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int    `json:"size"`
}

// ImageSetConfig used to decode command line 'imagesetconfig.yaml' file
type ImageSetConfig struct {
	Kind          string `yaml:"kind"`
	APIVersion    string `yaml:"apiVersion"`
	StorageConfig struct {
		Registry struct {
			ImageURL string `yaml:"imageURL"`
			SkipTLS  bool   `yaml:"skipTLS"`
		} `yaml:"registry"`
		Local struct {
			Path interface{} `yaml:"path"`
		} `yaml:"local"`
	} `yaml:"storageConfig"`
	Mirror struct {
		Platform struct {
			Channels []struct {
				Name string `yaml:"name"`
				Type string `yaml:"type"`
			} `yaml:"channels"`
		} `yaml:"platform"`
		Operators []struct {
			Catalog  string `yaml:"catalog"`
			Packages []struct {
				Name     string `yaml:"name"`
				Channels []struct {
					Name string `yaml:"name"`
				} `yaml:"channels"`
			} `yaml:"packages"`
		} `yaml:"operators"`
		AdditionalImages []struct {
			Name string `yaml:"name"`
		} `yaml:"additionalImages"`
		Helm struct {
		} `yaml:"helm"`
	} `yaml:"mirror"`
}

// ImageConfigJSON for each operator\the catalog or config json file
// for FBC layout
type ImageConfigJSON struct {
	Schema     string `json:"schema"`
	Name       string `json:"name"`
	Package    string `json:"package"`
	Image      string `json:"image"`
	Properties []struct {
		Type  string `json:"type"`
		Value struct {
			Group   string `json:"group"`
			Kind    string `json:"kind"`
			Version string `json:"version"`
		} `json:"value,omitempty"`
	} `json:"properties"`
	RelatedImages []struct {
		Name  string `json:"name"`
		Image string `json:"image"`
	} `json:"relatedImages"`
}

// UntarLayers simple function that untars the layer that
// has the FB configuration
func UntarLayers(gzipStream io.Reader, path string) error {
	uncompressedStream, err := gzip.NewReader(gzipStream)
	if err != nil {
		log.Fatal("UntarLayers: NewReader failed")
		return err
	}

	tarReader := tar.NewReader(uncompressedStream)

	for true {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			log.Fatalf("UntarLayers: Next() failed: %s", err.Error())
		}

		if strings.Contains(header.Name, "configs") {
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
				fmt.Println(fmt.Printf("UntarLayers: uknown type: %v in %s", header.Typeflag, header.Name))
			}
		}
	}
	return nil
}

// newSystemContext set the context for source & destination resources
func newSystemContext() *types.SystemContext {
	ctx := &types.SystemContext{
		RegistriesDirPath:           "",
		ArchitectureChoice:          "",
		OSChoice:                    "",
		VariantChoice:               "",
		BigFilesTemporaryDir:        "", //*globalArgs.cache + "/tmp",
		DockerInsecureSkipTLSVerify: types.OptionalBoolTrue,
	}
	return ctx
}

// getISConfig - simple function to read and unmarshal the imagesetconfig
// set via the command line
func (o *MirrorOptions) getISConfig() (*ImageSetConfig, error) {
	var isc *ImageSetConfig
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
func bulkImageCopy(isc *ImageSetConfig) error {

	files, err := ioutil.ReadDir(tempPath + configPath)
	if err != nil {
		log.Fatal(err)
		return err
	}

	ch := make(chan byte, 1)
	for _, pkg := range isc.Mirror.Operators[0].Packages {
		for _, file := range files {
			if strings.Contains(pkg.Name, file.Name()) {
				fmt.Println(file.Name(), file.IsDir())
				// read the config.json to get releated images
				icJSON, err := getRelatedImages(tempPath + configPath + file.Name())
				if err != nil {
					return err
				}
				for _, i := range icJSON.RelatedImages {
					go func() {
						name := i.Name
						if name == "" {
							name = "bundle"
						}
						err := copyImage(dockerProtocol+i.Image, ociProtocol+tempPath+configPath+file.Name()+"/"+name)
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
func bulkImageMirror(isc *ImageSetConfig, imgdest, namespace string) error {

	ch := make(chan byte, 1)
	for _, pkg := range isc.Mirror.Operators[0].Packages {
		icJSON, err := getRelatedImages(tempPath + configPath + pkg.Name)
		if err != nil {
			log.Fatal(err)
			return err
		}

		for _, i := range icJSON.RelatedImages {
			go func() {
				folder := i.Name
				if folder == "" {
					folder = "bundle"
				}
				tmp := strings.Split(i.Image, "/")
				fmt.Println("DEBUG LMZ ", tmp)
				img := strings.Split(tmp[2], ":")
				nm := strings.Split(img[0], "@")
				from := ociProtocol + tempPath + configPath + pkg.Name + "/" + folder
				to := dockerProtocol + imgdest + "/" + namespace + "/" + tmp[1] + "/" + nm[0] + ":v0.0.1"
				fmt.Println("copyImage(" + from + "," + to)
				err := copyImage(from, to)
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
func copyImage(from, to string) error {

	sourceCtx := newSystemContext()
	destinationCtx := newSystemContext()
	ctx := context.Background()

	// Pull the source image, and store it in the local storage, under the name main
	policy, err := signature.DefaultPolicy(nil)
	policyContext, err := signature.NewPolicyContext(policy)

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
	var index *Index

	// read the index.json of the catalog
	var oci *OCIImageManifest
	indexData, err := ioutil.ReadFile(path + indexJSON)
	if err != nil {
		return err
	}
	err = json.Unmarshal(indexData, &index)
	if err != nil {
		return err
	}

	// find the layer that is designated as the manifest
	manifestDigest := index.Manifests[0].Digest[7:]
	data, err := ioutil.ReadFile(path + blobsPath + manifestDigest)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, &oci)
	if err != nil {
		return err
	}

	// iterate through each layer

	for _, layer := range oci.Layers {
		fmt.Println(path + blobsPath + layer.Digest[7:])
		r, err := os.Open(path + blobsPath + layer.Digest[7:])
		if err != nil {
			return err
		}
		// untar if it is the FBC
		UntarLayers(r, tempPath)
	}
	return nil
}

// getRelatedImages this reads each catalog or config.json
// file in a given operator in the FBC
func getRelatedImages(path string) (*ImageConfigJSON, error) {
	var icJSON *ImageConfigJSON

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
	return icJSON, nil
}
