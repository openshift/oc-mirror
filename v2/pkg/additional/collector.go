package additional

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
)

const (
	indexJson               string = "manifest.json"
	operatorImageExtractDir string = "hold-operator"
	workingDir              string = "working-dir/"
	dockerProtocol          string = "docker://"
	ociProtocol             string = "oci://"
	ociProtocolTrimmed      string = "oci:"
	dirProtocol             string = "dir://"
	dirProtocolTrimmed      string = "dir:"
	additionalImagesDir     string = "additional-images"
	blobsDir                string = "/blobs/sha256/"
	diskToMirror            string = "diskToMirror"
	mirrorToDisk            string = "mirrorToDisk"
	errMsg                  string = "[AdditionalImagesCollector] %v "
	logsFile                string = "logs/additional-images.log"
)

type CollectorInterface interface {
	AdditionalImagesCollector(ctx context.Context) ([]v1alpha3.CopyImageSchema, error)
}

func New(log clog.PluggableLoggerInterface,
	config v1alpha2.ImageSetConfiguration,
	opts mirror.CopyOptions,
	mirror mirror.MirrorInterface,
	manifest manifest.ManifestInterface,
) CollectorInterface {
	return &Collector{Log: log, Config: config, Opts: opts, Mirror: mirror, Manifest: manifest}
}

type Collector struct {
	Log      clog.PluggableLoggerInterface
	Mirror   mirror.MirrorInterface
	Manifest manifest.ManifestInterface
	Config   v1alpha2.ImageSetConfiguration
	Opts     mirror.CopyOptions
}

// AdditionalImagesCollector - this looks into the additional images field
// taking into account the mode we are in (mirrorToDisk, diskToMirror)
// the image is downloaded in oci format
func (o *Collector) AdditionalImagesCollector(ctx context.Context) ([]v1alpha3.CopyImageSchema, error) {

	var allImages []v1alpha3.CopyImageSchema

	if o.Opts.Mode == mirrorToDisk {
		for _, img := range o.Config.ImageSetConfigurationSpec.Mirror.AdditionalImages {
			irs, err := customImageParser(img.Name)
			if err != nil {
				return []v1alpha3.CopyImageSchema{}, fmt.Errorf(errMsg, err)
			}
			cacheDir := strings.Join([]string{o.Opts.Global.Dir, additionalImagesDir, irs.Namespace, irs.Component}, "/")
			if _, err := os.Stat(cacheDir); errors.Is(err, os.ErrNotExist) {
				err := os.MkdirAll(cacheDir, 0755)
				if err != nil {
					return []v1alpha3.CopyImageSchema{}, nil
				}
				src := dockerProtocol + img.Name
				transport := strings.Split(o.Opts.Destination, "://")[0] + ":"
				dest := transport + cacheDir
				o.Log.Debug("source %s", src)
				o.Log.Debug("destination %s", dest)
				allImages = append(allImages, v1alpha3.CopyImageSchema{Source: src, Destination: dest})
			} else {
				o.Log.Info("cache dir exists %s", cacheDir)
			}
		}
	}

	if o.Opts.Mode == diskToMirror {
		regex, e := regexp.Compile(indexJson)
		if e != nil {
			o.Log.Error("%v", e)
		}
		for _, addImg := range o.Config.Mirror.AdditionalImages {
			imagesDir := strings.Replace(addImg.Name, "dir://", "", 1)
			e = filepath.Walk(imagesDir, func(path string, info os.FileInfo, err error) error {
				if err == nil && regex.MatchString(info.Name()) {
					hld := strings.Split(filepath.Dir(path), additionalImagesDir)
					//ref := filepath.Dir(strings.Join(hld, "/"))
					src := dirProtocolTrimmed + filepath.Dir(path)
					dest := o.Opts.Destination + hld[1]
					allImages = append(allImages, v1alpha3.CopyImageSchema{Source: src, Destination: dest})
				}
				return nil
			})
		}
		if e != nil {
			return []v1alpha3.CopyImageSchema{}, e
		}
	}
	return allImages, nil
}

// customImageParser - simple image string parser
func customImageParser(image string) (*v1alpha3.ImageRefSchema, error) {
	var irs *v1alpha3.ImageRefSchema
	var component string
	parts := strings.Split(image, "/")
	if len(parts) < 3 {
		return irs, fmt.Errorf("[customImageParser] image url seems to be wrong %s ", image)
	}
	component = parts[2]
	if strings.Contains(parts[2], "@") {
		component = strings.Split(parts[2], "@")[0]
	}
	if strings.Contains(parts[2], ":") {
		component = strings.Split(parts[2], ":")[0]
	}
	irs = &v1alpha3.ImageRefSchema{Repository: parts[0], Namespace: parts[1], Component: component}
	return irs, nil
}
