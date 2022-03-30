package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
)

// TODO(estroz): create interface scheme such that configuration and metadata
// versions do not matter to the caller.
// See https://github.com/kubernetes-sigs/controller-runtime/blob/master/pkg/config/config.go

func ReadConfig(configPath string) (c v1alpha2.ImageSetConfiguration, err error) {

	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return c, err
	}
	typeMeta, err := getTypeMeta(data)

	if err != nil {
		return c, err
	}

	switch typeMeta.GroupVersionKind() {
	case v1alpha2.GroupVersion.WithKind(v1alpha2.ImageSetConfigurationKind):
		c, err = LoadConfig(data)
		if err != nil {
			return c, err
		}
	default:
		return c, fmt.Errorf("config GVK not recognized: %s", typeMeta.GroupVersionKind())
	}

	return c, Validate(&c)
}

func LoadConfig(data []byte) (c v1alpha2.ImageSetConfiguration, err error) {

	gvk := v1alpha2.GroupVersion.WithKind(v1alpha2.ImageSetConfigurationKind)

	if data, err = yaml.YAMLToJSON(data); err != nil {
		return c, fmt.Errorf("yaml to json %s: %v", gvk, err)
	}

	dec := json.NewDecoder(bytes.NewBuffer(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("decode %s: %v", gvk, err)
	}

	c.SetGroupVersionKind(gvk)

	return c, nil
}

func LoadMetadata(data []byte) (m v1alpha2.Metadata, err error) {

	gvk := v1alpha2.GroupVersion.WithKind(v1alpha2.MetadataKind)

	dec := json.NewDecoder(bytes.NewBuffer(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, fmt.Errorf("decode %s: %v", gvk, err)
	}

	m.SetGroupVersionKind(gvk)

	return m, nil
}

func getTypeMeta(data []byte) (typeMeta metav1.TypeMeta, err error) {
	if err := yaml.Unmarshal(data, &typeMeta); err != nil {
		return typeMeta, fmt.Errorf("get type meta: %v", err)
	}
	return typeMeta, nil
}
