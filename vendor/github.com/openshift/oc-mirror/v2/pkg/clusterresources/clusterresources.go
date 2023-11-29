package clusterresources

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	confv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/openshift/oc-mirror/v2/pkg/mirror"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

const (
	clusterResourcesDir string = "cluster-resources"
)

func New(log clog.PluggableLoggerInterface,
	config v1alpha2.ImageSetConfiguration,
	opts mirror.CopyOptions,
) GeneratorInterface {
	return &ClusterResourcesGenerator{Log: log, Config: config, Opts: opts}
}

type ClusterResourcesGenerator struct {
	Log    clog.PluggableLoggerInterface
	Config v1alpha2.ImageSetConfiguration
	Opts   mirror.CopyOptions
}

func (c *ClusterResourcesGenerator) IDMSGenerator(ctx context.Context, allRelatedImages []v1alpha3.CopyImageSchema, opts mirror.CopyOptions) error {

	// determine the name of the IDMS resource
	// TODO determine name (based on date?)
	dateTime := time.Now().UTC().Format(time.RFC3339)
	// replace all : by -
	dateTime = strings.ReplaceAll(dateTime, ":", "-")
	dateTime = strings.ToLower(dateTime)
	name := "idms-" + dateTime

	// locate the output directory
	idmsFileName := filepath.Join(opts.Global.WorkingDir, clusterResourcesDir, name+".yaml")

	// create a IDMS struct
	idms := confv1.ImageDigestMirrorSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: confv1.GroupVersion.String(),
			Kind:       "ImageDigestMirrorSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: confv1.ImageDigestMirrorSetSpec{
			ImageDigestMirrors: []confv1.ImageDigestMirrors{},
		},
	}
	// populate IDMS from allRelatedImages
	mirrors, err := generateImageMirrors(allRelatedImages)
	if err != nil {
		return err
	}

	for source, imgMirrors := range mirrors {
		idm := confv1.ImageDigestMirrors{
			Source:  source,
			Mirrors: imgMirrors,
		}
		idms.Spec.ImageDigestMirrors = append(idms.Spec.ImageDigestMirrors, idm)
	}

	// put IDMS in yaml
	bytes, err := yaml.Marshal(idms)
	if err != nil {
		return err
	}

	// save IDMS struct to file
	if _, err := os.Stat(idmsFileName); errors.Is(err, os.ErrNotExist) {
		c.Log.Info("%s does not exist, creating it", idmsFileName)
		err := os.MkdirAll(filepath.Dir(idmsFileName), 0755)
		if err != nil {
			return err
		}
		c.Log.Info("%s dir created", filepath.Dir(idmsFileName))
	}
	idmsFile, err := os.Create(idmsFileName)
	if err != nil {
		return err
	}
	c.Log.Info("%s file created", idmsFileName)

	defer idmsFile.Close()

	_, err = idmsFile.Write(bytes)
	return err
}

func generateImageMirrors(allRelatedImages []v1alpha3.CopyImageSchema) (map[string][]confv1.ImageMirror, error) {
	mirrors := make(map[string][]confv1.ImageMirror, 0)

	for _, relatedImage := range allRelatedImages {
		if relatedImage.Origin == "" {
			return nil, fmt.Errorf("unable to generate IDMS/ITMS: original reference for (%s,%s) undetermined", relatedImage.Source, relatedImage.Destination)
		}
		// locate source namespace
		// strip away protocol
		originRef := relatedImage.Origin
		srcTransportAndPath := strings.Split(relatedImage.Origin, "://")
		if len(srcTransportAndPath) > 1 {
			originRef = srcTransportAndPath[1]
		}
		srcPathComponents := strings.Split(originRef, "/")
		srcNs := filepath.Join(srcPathComponents[:len(srcPathComponents)-1]...)

		// locate mirror namespace
		destRef := relatedImage.Destination
		// strip away protocol
		destTransportAndPath := strings.Split(destRef, "://")
		if len(destTransportAndPath) > 1 {
			destRef = destTransportAndPath[1]
		}
		destPathComponents := strings.Split(destRef, "/")
		destNs := filepath.Join(destPathComponents[:len(destPathComponents)-1]...)

		// add entry to map
		if mirrors[srcNs] == nil {
			mirrors[srcNs] = []confv1.ImageMirror{confv1.ImageMirror(destNs)}
		} else {
			alreadyAdded := false
			for _, m := range mirrors[srcNs] {
				if m == confv1.ImageMirror(destNs) {
					alreadyAdded = true
					break
				}
			}
			if !alreadyAdded {
				mirrors[srcNs] = append(mirrors[srcNs], confv1.ImageMirror(destNs))
			}
		}
	}
	return mirrors, nil
}
