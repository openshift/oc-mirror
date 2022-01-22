package mirror

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type ICSPType int

const (
	// Generic ICSP is the default type
	TypeGeneric ICSPType = iota
	TypeOCPRelease
	TypeOperator
)

// Copied from https://github.com/openshift/oc/blob/5d8dfa1c2e8e7469d69d76f21e0a166a0de8663b/pkg/cli/admin/catalog/mirror.go#L549
// Changes made are breaking ICSP and Catalog Source generation into different functions
type ICSPGenerator struct {
	ImageName   string
	ICSPMapping map[reference.DockerImageReference]reference.DockerImageReference
	ICSPType    ICSPType
}

func (g *ICSPGenerator) init() {
	if g.ICSPMapping == nil {
		g.ICSPMapping = make(map[reference.DockerImageReference]reference.DockerImageReference)
	}
}

func (g *ICSPGenerator) Run(icspScope string, byteLimit int) (icsps []operatorv1alpha1.ImageContentSourcePolicy, err error) {
	g.init()

	registryMapping := getRegistryMapping(icspScope, g.ICSPMapping)

	for icspCount := 0; len(registryMapping) != 0; icspCount++ {
		name := strings.Join(strings.Split(g.ImageName, "/"), "-") + "-" + strconv.Itoa(icspCount)
		icsp := operatorv1alpha1.ImageContentSourcePolicy{
			TypeMeta: metav1.TypeMeta{
				APIVersion: operatorv1alpha1.GroupVersion.String(),
				Kind:       "ImageContentSourcePolicy"},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: operatorv1alpha1.ImageContentSourcePolicySpec{
				RepositoryDigestMirrors: []operatorv1alpha1.RepositoryDigestMirrors{},
			},
		}

		if g.ICSPType == TypeOperator {
			icsp.Labels = map[string]string{
				"operators.openshift.org/catalog": "true",
			}
		}

		for key := range registryMapping {
			icsp.Spec.RepositoryDigestMirrors = append(icsp.Spec.RepositoryDigestMirrors, operatorv1alpha1.RepositoryDigestMirrors{
				Source:  key,
				Mirrors: []string{registryMapping[key]},
			})

			// FIXME(jpower432): add this as a workaround until
			// mirroring individual images for release is implemented
			// add OCP component image location to all release ICSPs
			if g.ICSPType == TypeOCPRelease {
				icsp.Spec.RepositoryDigestMirrors = append(icsp.Spec.RepositoryDigestMirrors, operatorv1alpha1.RepositoryDigestMirrors{
					Source:  "quay.io/openshift-release-dev/ocp-v4.0-art-dev",
					Mirrors: []string{registryMapping[key]},
				})
			}

			y, err := yaml.Marshal(icsp)
			if err != nil {
				return nil, fmt.Errorf("unable to marshal ImageContentSourcePolicy yaml: %v", err)
			}
			if len(y) > byteLimit {
				if lenMirrors := len(icsp.Spec.RepositoryDigestMirrors); lenMirrors > 0 {
					if lenMirrors == 1 {
						return nil, fmt.Errorf("repository digest mirror for %q cannot fit into any ICSP with byte limit %d", key, byteLimit)
					}
					icsp.Spec.RepositoryDigestMirrors = icsp.Spec.RepositoryDigestMirrors[:lenMirrors-1]
				}
				break
			}
			delete(registryMapping, key)
		}

		if len(icsp.Spec.RepositoryDigestMirrors) != 0 {
			icsps = append(icsps, icsp)
		}
	}

	return icsps, nil
}

func aggregateICSPs(icsps [][]byte) []byte {
	aggregation := []byte{}
	for _, icsp := range icsps {
		aggregation = append(aggregation, []byte("---\n")...)
		aggregation = append(aggregation, icsp...)
	}
	return aggregation
}

func getRegistryMapping(icspScope string, mapping map[reference.DockerImageReference]reference.DockerImageReference) map[string]string {
	registryMapping := map[string]string{}
	for k, v := range mapping {
		if len(v.ID) == 0 {
			logrus.Warnf("no digest mapping available for %s, skip writing to ImageContentSourcePolicy", k)
			continue
		}
		if icspScope == "registry" {
			registryMapping[k.Registry] = v.Registry
		} else {
			registryMapping[k.AsRepository().String()] = v.AsRepository().String()
		}
	}
	return registryMapping
}

func generateCatalogSource(name string, dest reference.DockerImageReference) ([]byte, error) {
	// Prefer tag over digest for automatic updates.
	if dest.Tag != "" {
		dest.ID = ""
	}

	obj := map[string]interface{}{
		"apiVersion": "operators.coreos.com/v1alpha1",
		"kind":       "CatalogSource",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": "openshift-marketplace",
		},
		"spec": map[string]interface{}{
			"sourceType": "grpc",
			"image":      dest.String(),
		},
	}
	cs, err := yaml.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal CatalogSource yaml: %v", err)
	}

	return cs, nil
}

func WriteICSPs(dir string, icsps []operatorv1alpha1.ImageContentSourcePolicy) error {

	if len(icsps) == 0 {
		logrus.Debug("No ICSPs generated to write")
		return nil
	}

	// Stable ICSP generation.
	sort.Slice(icsps, func(i, j int) bool {
		return string(icsps[i].Name) < string(icsps[j].Name)
	})

	icspBytes := make([][]byte, len(icsps))
	for i, icsp := range icsps {
		// Create an unstructured object for removing creationTimestamp
		obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&icsp)
		if err != nil {
			return fmt.Errorf("error converting to unstructured: %v", err)
		}
		delete(obj["metadata"].(map[string]interface{}), "creationTimestamp")

		if icspBytes[i], err = yaml.Marshal(obj); err != nil {
			return fmt.Errorf("unable to marshal ImageContentSourcePolicy yaml: %v", err)
		}
	}

	if err := ioutil.WriteFile(filepath.Join(dir, "imageContentSourcePolicy.yaml"), aggregateICSPs(icspBytes), os.ModePerm); err != nil {
		return fmt.Errorf("error writing ImageContentSourcePolicy: %v", err)
	}

	logrus.Infof("Wrote ICSP manifests to %s", dir)

	return nil
}

func WriteCatalogSource(source, dest reference.DockerImageReference, dir string) error {

	name := source.Name
	catalogSource, err := generateCatalogSource(name, dest)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(filepath.Join(dir, fmt.Sprintf("catalogSource-%s.yaml", name)), catalogSource, os.ModePerm); err != nil {
		return fmt.Errorf("error writing CatalogSource: %v", err)
	}

	logrus.Infof("Wrote CatalogSource manifests to %s", dir)

	return nil
}
