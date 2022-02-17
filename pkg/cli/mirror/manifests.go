package mirror

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"regexp"
	"net/http"
	"errors"
	"io"
	b64 "encoding/base64"

	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openshift/oc-mirror/pkg/image"
)

const (
	icspSizeLimit       = 250000
	registryICSPScope   = "registry"
	repositoryICSPScope = "repository"
	namespaceICSPScope  = "namespace"
	icspKind            = "ImageContentSourcePolicy"
	digestAlgo          = "sha256"
	releaseSignatureURL = "https://mirror.openshift.com/pub/openshift-v4/signatures/openshift/release/%s=%s/signature-1"
)

var icspTypeMeta = metav1.TypeMeta{
	APIVersion: operatorv1alpha1.GroupVersion.String(),
	Kind:       icspKind,
}

// ICSPBuilder defines methods for generating ICSPs
type ICSPBuilder interface {
	New(string, int) operatorv1alpha1.ImageContentSourcePolicy
	GetMapping(string, image.TypedImageMapping) (map[string]string, error)
}

var _ ICSPBuilder = &ReleaseBuilder{}

type ReleaseBuilder struct{}

func (b *ReleaseBuilder) New(icspName string, icspCount int) operatorv1alpha1.ImageContentSourcePolicy {
	name := strings.Join(strings.Split(icspName, "/"), "-") + "-" + strconv.Itoa(icspCount)
	return operatorv1alpha1.ImageContentSourcePolicy{
		TypeMeta: icspTypeMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: operatorv1alpha1.ImageContentSourcePolicySpec{
			RepositoryDigestMirrors: []operatorv1alpha1.RepositoryDigestMirrors{},
		},
	}
}

func (b *ReleaseBuilder) GetMapping(_ string, mapping image.TypedImageMapping) (map[string]string, error) {
	// Scope is set to repository for release because
	// they are mirrored as different repo names by
	// release planner
	return getRegistryMapping(repositoryICSPScope, mapping)
}

var _ ICSPBuilder = &OperatorBuilder{}

type OperatorBuilder struct{}

func (b *OperatorBuilder) New(icspName string, icspCount int) operatorv1alpha1.ImageContentSourcePolicy {
	name := strings.Join(strings.Split(icspName, "/"), "-") + "-" + strconv.Itoa(icspCount)
	return operatorv1alpha1.ImageContentSourcePolicy{
		TypeMeta: icspTypeMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"operators.openshift.org/catalog": "true"},
		},
		Spec: operatorv1alpha1.ImageContentSourcePolicySpec{
			RepositoryDigestMirrors: []operatorv1alpha1.RepositoryDigestMirrors{},
		},
	}
}

func (b *OperatorBuilder) GetMapping(icspScope string, mapping image.TypedImageMapping) (map[string]string, error) {
	return getRegistryMapping(icspScope, mapping)
}

var _ ICSPBuilder = &GenericBuilder{}

type GenericBuilder struct{}

func (b *GenericBuilder) New(icspName string, icspCount int) operatorv1alpha1.ImageContentSourcePolicy {
	name := strings.Join(strings.Split(icspName, "/"), "-") + "-" + strconv.Itoa(icspCount)
	return operatorv1alpha1.ImageContentSourcePolicy{
		TypeMeta: icspTypeMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: operatorv1alpha1.ImageContentSourcePolicySpec{
			RepositoryDigestMirrors: []operatorv1alpha1.RepositoryDigestMirrors{},
		},
	}
}

func (b *GenericBuilder) GetMapping(icspScope string, mapping image.TypedImageMapping) (map[string]string, error) {
	return getRegistryMapping(icspScope, mapping)
}

// GenerateICSP will generate ImageContentSourcePolicy objects based on image mapping and an ICSPBuilder
func GenerateICSP(icspName, icspScope string, byteLimit int, mapping image.TypedImageMapping, builder ICSPBuilder) (icsps []operatorv1alpha1.ImageContentSourcePolicy, err error) {
	registryMapping, err := builder.GetMapping(icspScope, mapping)
	if err != nil {
		return nil, err
	}

	for len(registryMapping) != 0 {

		var icspCount int
		icsp := builder.New(icspName, icspCount)

		for key := range registryMapping {
			icsp.Spec.RepositoryDigestMirrors = append(icsp.Spec.RepositoryDigestMirrors, operatorv1alpha1.RepositoryDigestMirrors{
				Source:  key,
				Mirrors: []string{registryMapping[key]},
			})

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
				icspCount++
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

func getRegistryMapping(icspScope string, mapping image.TypedImageMapping) (map[string]string, error) {
	registryMapping := map[string]string{}
	for k, v := range mapping {
		if len(v.Ref.ID) == 0 {
			logrus.Warnf("no digest mapping available for %s, skip writing to ImageContentSourcePolicy", k)
			continue
		}
		switch {
		case icspScope == registryICSPScope:
			registryMapping[k.Ref.Registry] = v.Ref.Registry
		case icspScope == namespaceICSPScope && k.Ref.Namespace == "":
			fallthrough
		case icspScope == repositoryICSPScope:
			registryMapping[k.Ref.AsRepository().String()] = v.Ref.AsRepository().String()
		case icspScope == namespaceICSPScope:
			source := path.Join(k.Ref.Registry, k.Ref.Namespace)
			dest := path.Join(v.Ref.Registry, v.Ref.Namespace)
			registryMapping[source] = dest
		default:
			return registryMapping, fmt.Errorf("invalid ICSP scope %s", icspScope)
		}
	}

	return registryMapping, nil
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

// WriteICSPs will write provided ImageContentSourcePolicy objects to disk
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

// WriteCatalogSource will generate a CatalogSource object and write it to disk
func WriteCatalogSource(mapping image.TypedImageMapping, dir string) error {
	for source, dest := range mapping {
		name := source.Ref.Name
		catalogSource, err := generateCatalogSource(name, dest.Ref)
		if err != nil {
			return err
		}
		if err := ioutil.WriteFile(filepath.Join(dir, fmt.Sprintf("catalogSource-%s.yaml", name)), catalogSource, os.ModePerm); err != nil {
			return fmt.Errorf("error writing CatalogSource: %v", err)
		}
	}
	logrus.Infof("Wrote CatalogSource manifests to %s", dir)
	return nil
}

// WriteReleaseSignature will generate a ConfigMap used during upgrading and write it to disk
func WriteReleaseSignature(image string, version string, dir string) error {
	releaseConfigMap, err := generateReleaseSignature(image, version)
	if err != nil {
		logrus.Errorf("Error from generateReleaseSignature %v", err)
		return err
	}
	if err := ioutil.WriteFile(filepath.Join(dir, fmt.Sprintf("release-image-%s-configmap.yaml", version)), releaseConfigMap, os.ModePerm); err != nil {
		return fmt.Errorf("error writing ReleaseConfigMap: %v", err)
	}

	return nil
}

func generateReleaseSignature(image string, version string) ([]byte, error) {
	name := "release-image-"+version

	replace := regexp.MustCompile(`.*:`)
	digest := replace.ReplaceAllString(image, "")
	url := fmt.Sprintf(releaseSignatureURL, digestAlgo, digest)

	response, err := http.Get(url)
	if err != nil {
		logrus.Errorf("Error getting signature for %s, error was %v", url, err)
		return nil, err
	}

	defer response.Body.Close()


	if response.StatusCode != http.StatusOK {
		return nil, errors.New(fmt.Sprintf("Release signature not found for release, tried to get %s and http code was %s", url, strconv.Itoa(response.StatusCode)))
	}


	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		logrus.Errorf("Error reading response body, erorr was %v", err)
		return nil, err
	}

	digestB64 := b64.StdEncoding.EncodeToString(bodyBytes)

	obj := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": "openshift-config-managed",
			"labels": map[string]interface{}{
				"release.openshift.io/verification-signatures": "",
			},
		},
		"binaryData": map[string]interface{}{
			digestAlgo+"-"+digest: digestB64,
		},
	}

	cm, err := yaml.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal ReleaseConfigMap yaml: %v", err)
	}

	return cm, nil
}