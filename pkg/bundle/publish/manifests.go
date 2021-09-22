package publish

import (
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// Copied from https://github.com/openshift/oc/blob/5d8dfa1c2e8e7469d69d76f21e0a166a0de8663b/pkg/cli/admin/catalog/mirror.go#L549
// Changes made are breaking ICSP and Catalog Source generation into different functions
func WriteICSPs(dir string, icsps []operatorv1alpha1.ImageContentSourcePolicy) error {

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

func WriteCatalogSource(source imagesource.TypedImageReference, dir string, mapping map[imagesource.TypedImageReference]imagesource.TypedImageReference) error {

	dest, ok := mapping[source]
	if !ok {
		return fmt.Errorf("no mapping found for index image")
	}

	return writeCatalogSource(source, dest, dir)
}

func writeCatalogSource(source, dest imagesource.TypedImageReference, dir string) error {

	catalogSource, err := generateCatalogSource(source, dest)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(filepath.Join(dir, fmt.Sprintf("catalogSource-%s.yaml", source.Ref.Name)), catalogSource, os.ModePerm); err != nil {
		return fmt.Errorf("error writing CatalogSource: %v", err)
	}

	logrus.Infof("Wrote CatalogSource manifests to %s", dir)

	return nil
}

func GenerateICSPs(imageName string, byteLimit int, icspScope string, mapping map[reference.DockerImageReference]reference.DockerImageReference) (icsps []operatorv1alpha1.ImageContentSourcePolicy, err error) {

	registryMapping := getRegistryMapping(icspScope, mapping)

	for icspCount := 0; len(registryMapping) != 0; icspCount++ {
		name := strings.Join(strings.Split(imageName, "/"), "-") + "-" + strconv.Itoa(icspCount)
		icsp := operatorv1alpha1.ImageContentSourcePolicy{
			TypeMeta: metav1.TypeMeta{
				APIVersion: operatorv1alpha1.GroupVersion.String(),
				Kind:       "ImageContentSourcePolicy"},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Labels: map[string]string{
					"operators.openshift.org/catalog": "true",
				},
			},
			Spec: operatorv1alpha1.ImageContentSourcePolicySpec{
				RepositoryDigestMirrors: []operatorv1alpha1.RepositoryDigestMirrors{},
			},
		}

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

func generateCatalogSource(source, dest imagesource.TypedImageReference) ([]byte, error) {
	unstructuredObj := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "operators.coreos.com/v1alpha1",
			"kind":       "CatalogSource",
			"metadata": map[string]interface{}{
				"name":      source.Ref.Name,
				"namespace": "openshift-marketplace",
			},
			"spec": map[string]interface{}{
				"sourceType": "grpc",
				"image":      dest.String(),
			},
		},
	}
	csExample, err := yaml.Marshal(unstructuredObj.Object)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal CatalogSource yaml: %v", err)
	}

	return csExample, nil
}

// Copied from https://github.com/openshift/oc/blob/183090787d00d5150440dd75d71b00a72739a86c/pkg/cli/admin/catalog/mirrorer.go#L45
// Only used to create manifest mapping without actually mirroring
func mappingForImages(images map[string]struct{}, src, dest imagesource.TypedImageReference, maxComponents int) (mapping map[imagesource.TypedImageReference]imagesource.TypedImageReference, errs []error) {
	if dest.Type != imagesource.DestinationRegistry {
		// don't do any name mangling when not mirroring to a real registry
		// this allows us to assume the names are preserved when doing multi-hop mirrors that use a file or s3 as an
		// intermediate step
		maxComponents = 0

		// if mirroring a source (like quay.io/my/index:1) to a file location like file://local/store
		// we will remount all of the content in the file store under the catalog name
		// i.e. file://local/store/my/index
		var err error
		dest, err = mount(src, dest, 0)
		if err != nil {
			errs = []error{err}
			return
		}
	}

	mapping = map[imagesource.TypedImageReference]imagesource.TypedImageReference{}
	for img := range images {
		if img == "" {
			continue
		}

		parsed, err := imagesource.ParseReference(img)
		if err != nil {
			errs = append(errs, fmt.Errorf("couldn't parse image for mirroring (%s), skipping mirror: %v", img, err))
			continue
		}

		targetRef, err := mount(parsed, dest, maxComponents)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		// set docker defaults, but don't set default tag for digest refs
		s := parsed
		parsed.Ref = parsed.Ref.DockerClientDefaults()
		if len(s.Ref.Tag) == 0 && len(s.Ref.ID) > 0 {
			parsed.Ref.Tag = ""
		}

		// if src is a file store, assume all other references are in the same location on disk
		if src.Type != imagesource.DestinationRegistry {
			srcRef, err := mount(parsed, src, 0)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if len(parsed.Ref.Tag) == 0 {
				srcRef.Ref.Tag = ""
			}
			mapping[srcRef] = targetRef
			continue
		}

		mapping[parsed] = targetRef
	}
	return
}

func mount(in, dest imagesource.TypedImageReference, maxComponents int) (out imagesource.TypedImageReference, err error) {
	out = in
	out.Type = dest.Type

	hasher := fnv.New32a()
	// tag with hash of source ref if no tag given
	if len(out.Ref.Tag) == 0 && len(out.Ref.ID) > 0 {
		hasher.Reset()
		_, err = hasher.Write([]byte(out.Ref.String()))
		if err != nil {
			err = fmt.Errorf("couldn't generate tag for image (%s), skipping mirror", in.String())
		}
		out.Ref.Tag = fmt.Sprintf("%x", hasher.Sum32())
	}

	// fill in default registry / tag if missing
	out.Ref = out.Ref.DockerClientDefaults()

	components := []string{}
	if len(dest.Ref.Namespace) > 0 {
		components = append(components, dest.Ref.Namespace)
	}
	if len(dest.Ref.Name) > 0 {
		components = append(components, strings.Split(dest.Ref.Name, "/")...)
	}
	if len(out.Ref.Namespace) > 0 {
		components = append(components, out.Ref.Namespace)
	}
	if len(out.Ref.Name) > 0 {
		components = append(components, strings.Split(out.Ref.Name, "/")...)
	}

	out.Ref.Registry = dest.Ref.Registry
	out.Ref.Namespace = components[0]
	if maxComponents > 1 && len(components) > maxComponents {
		out.Ref.Name = strings.Join(components[1:maxComponents-1], "/") + "/" + strings.Join(components[maxComponents-1:], "-")
	} else if maxComponents == 0 {
		out.Ref.Name = strings.Join(components[1:], "/")
	} else if len(components) > 1 {
		endIndex := maxComponents
		if endIndex > len(components) {
			endIndex = len(components)
		}

		out.Ref.Name = strings.Join(components[1:endIndex], "/")
	} else {
		// only one component, make it the name, not the namespace
		out.Ref.Name = in.Ref.Name
		out.Ref.Namespace = ""
	}
	out.Ref.Name = strings.TrimPrefix(out.Ref.Name, "/")
	return
}
