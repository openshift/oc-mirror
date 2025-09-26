package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

var (
	errMissingMirrorStanza = errors.New("configuration missing the `mirror` stanza")
	errMissingDeleteStanza = errors.New("configuration missing the `delete` stanza")
	errMissingKind         = errors.New("configuration missing `kind`")
)

// ReadConfig opens an imageset configuration file at the given path
// and loads it into a v2alpha1.ImageSetConfiguration instance for processing and validation.
func ReadConfig(configPath string, kind string) (interface{}, error) {
	data, err := os.ReadFile(filepath.Clean(configPath))
	if err != nil {
		return nil, fmt.Errorf("could not parse config: %w", err)
	}

	var configMap map[string]any
	if err := yaml.UnmarshalStrict(data, &configMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	configKind, ok := configMap["kind"]
	if !ok {
		return nil, errMissingKind
	}
	if configKind != kind {
		return nil, fmt.Errorf("cannot parse %q as %q", configKind, kind)
	}
	switch kind {
	case v2alpha1.ImageSetConfigurationKind:
		if _, ok := configMap["mirror"]; !ok {
			return nil, errMissingMirrorStanza
		}
		if _, ok := configMap["delete"]; ok {
			return nil, fmt.Errorf("delete: is not allowed in %s", kind)
		}
		cfg, err := LoadConfig[v2alpha1.ImageSetConfiguration](data, v2alpha1.ImageSetConfigurationKind)
		gvk := v2alpha1.GroupVersion.WithKind(v2alpha1.ImageSetConfigurationKind)
		cfg.SetGroupVersionKind(gvk)
		if err != nil {
			return nil, err
		}
		Complete(&cfg)
		err = Validate(&cfg)
		if err != nil {
			return nil, err
		}
		return cfg, nil
	case v2alpha1.DeleteImageSetConfigurationKind:
		if _, ok := configMap["delete"]; !ok {
			return nil, errMissingDeleteStanza
		}
		if _, ok := configMap["mirror"]; ok {
			return nil, fmt.Errorf("mirror: is not allowed in %s", kind)
		}
		cfg, err := LoadConfig[v2alpha1.DeleteImageSetConfiguration](data, v2alpha1.DeleteImageSetConfigurationKind)
		gvk := v2alpha1.GroupVersion.WithKind(v2alpha1.DeleteImageSetConfigurationKind)
		cfg.SetGroupVersionKind(gvk)
		if err != nil {
			return nil, err
		}
		CompleteDelete(&cfg)
		err = ValidateDelete(&cfg)
		if err != nil {
			return nil, err
		}
		return cfg, nil
	default:
		return nil, fmt.Errorf("config kind %q not supported", kind)
	}
}

// LoadConfig loads data into a v2alpha1.ImageSetConfiguration or
// v2alpha1.DeleteImageSetConfiguration instance
func LoadConfig[T any](data []byte, kind string) (c T, err error) {
	if data, err = yaml.YAMLToJSON(data); err != nil {
		return c, fmt.Errorf("yaml to json %s: %v", kind, err)
	}

	var res T
	dec := json.NewDecoder(bytes.NewBuffer(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&res); err != nil {
		return c, fmt.Errorf("decode %s: %v", kind, err)
	}
	return res, nil
}

// LoadConfigDelete loads data into a v2alpha1.ImageSetConfiguration instance
func LoadConfigDelete(data []byte) (c v2alpha1.DeleteImageSetConfiguration, err error) {
	gvk := v2alpha1.GroupVersion.WithKind(v2alpha1.DeleteImageSetConfigurationKind)

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
