package publish

import (
	"fmt"
	"hash/fnv"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	"github.com/openshift/oc/pkg/cli/image/imagesource"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// Copied from https://github.com/openshift/oc/blob/5d8dfa1c2e8e7469d69d76f21e0a166a0de8663b/pkg/cli/admin/catalog/mirror.go#L549
// Changes made are breaking ICSP and Catalog Source generation into different functions
func WriteICSP(out io.Writer, dir string, icsps [][]byte) error {

	if err := ioutil.WriteFile(filepath.Join(dir, "imageContentSourcePolicy.yaml"), aggregateICSPs(icsps), os.ModePerm); err != nil {
		return fmt.Errorf("error writing ImageContentSourcePolicy")
	}

	fmt.Fprintf(out, "wrote ICSP manifests to %s\n", dir)

	return nil
}

func WriteCatalogSource(out io.Writer, source imagesource.TypedImageReference, dir string, mapping map[imagesource.TypedImageReference]imagesource.TypedImageReference) error {

	dest, ok := mapping[source]
	if !ok {
		return fmt.Errorf("no mapping found for index image")
	}

	return writeCatalogSource(out, source, dest, dir)
}

func writeCatalogSource(out io.Writer, source, dest imagesource.TypedImageReference, dir string) error {

	catalogSource, err := generateCatalogSource(source, dest)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(filepath.Join(dir, fmt.Sprintf("catalogSource-%s.yaml", source.Ref.Name)), catalogSource, os.ModePerm); err != nil {
		return fmt.Errorf("error writing CatalogSource")
	}

	fmt.Fprintf(out, "wrote CatalogSource manifests to %s\n", dir)

	return nil
}

func GenerateICSP(out io.Writer, name string, byteLimit int, icspScope string, mapping map[imagesource.TypedImageReference]imagesource.TypedImageReference) ([]byte, error) {
	registryMapping := getRegistryMapping(out, icspScope, mapping)
	icsp := operatorv1alpha1.ImageContentSourcePolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: operatorv1alpha1.GroupVersion.String(),
			Kind:       "ImageContentSourcePolicy"},
		ObjectMeta: metav1.ObjectMeta{
			Name: strings.Join(strings.Split(name, "/"), "-"),
			Labels: map[string]string{
				"operators.openshift.org/catalog": "true",
			},
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
				icsp.Spec.RepositoryDigestMirrors = icsp.Spec.RepositoryDigestMirrors[:lenMirrors-1]
			}
			break
		}
	}

	// Create an unstructured object for removing creationTimestamp
	unstructuredObj := unstructured.Unstructured{}
	var err error
	unstructuredObj.Object, err = runtime.DefaultUnstructuredConverter.ToUnstructured(&icsp)
	if err != nil {
		return nil, fmt.Errorf("error converting to unstructured: %v", err)
	}
	delete(unstructuredObj.Object["metadata"].(map[string]interface{}), "creationTimestamp")

	icspExample, err := yaml.Marshal(unstructuredObj.Object)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal ImageContentSourcePolicy yaml: %v", err)
	}

	return icspExample, nil
}

func aggregateICSPs(icsps [][]byte) []byte {
	aggregation := []byte{}
	for _, icsp := range icsps {
		aggregation = append(aggregation, []byte("---\n")...)
		aggregation = append(aggregation, icsp...)
	}
	return aggregation
}

func getRegistryMapping(out io.Writer, icspScope string, mapping map[imagesource.TypedImageReference]imagesource.TypedImageReference) map[string]string {
	registryMapping := map[string]string{}
	for k, v := range mapping {
		if len(v.Ref.ID) == 0 {
			fmt.Fprintf(out, "no digest mapping available for %s, skip writing to ImageContentSourcePolicy\n", k)
			continue
		}
		if icspScope == "registry" {
			registryMapping[k.Ref.Registry] = v.Ref.Registry
		} else {
			registryMapping[k.Ref.AsRepository().String()] = v.Ref.AsRepository().String()
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
