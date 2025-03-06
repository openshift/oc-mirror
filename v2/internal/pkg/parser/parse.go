//nolint:ireturn // generic T should be fine to return here
package parser

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"sigs.k8s.io/yaml"
)

// ParseJsonFile reads json `file` and parses it as type T
func ParseJsonFile[T any](file string) (T, error) {
	var t T
	data, err := os.ReadFile(file)
	if err != nil {
		return t, fmt.Errorf("read file: %w", err)
	}
	if err := json.Unmarshal(data, &t); err != nil {
		return t, fmt.Errorf("parse file: %w", err)
	}
	return t, nil
}

// ParseJsonReader parses json from a stream into a type T
func ParseJsonReader[T any](r io.Reader) (T, error) {
	var t T
	data, err := io.ReadAll(r)
	if err != nil {
		return t, fmt.Errorf("read stream: %w", err)
	}
	if err := json.Unmarshal(data, &t); err != nil {
		return t, fmt.Errorf("parse stream: %w", err)
	}
	return t, nil
}

// ParseYamlFile reads yaml `file` and parses it as type T
func ParseYamlFile[T any](file string) (T, error) {
	var t T
	data, err := os.ReadFile(file)
	if err != nil {
		return t, fmt.Errorf("read file: %w", err)
	}
	if err := yaml.Unmarshal(data, &t); err != nil {
		return t, fmt.Errorf("parse file: %w", err)
	}
	return t, nil
}

// ParseYamlReader parses yaml from a stream into a type T
func ParseYamlReader[T any](r io.Reader) (T, error) {
	var t T
	data, err := io.ReadAll(r)
	if err != nil {
		return t, fmt.Errorf("read stream: %w", err)
	}
	if err := yaml.Unmarshal(data, &t); err != nil {
		return t, fmt.Errorf("parse stream: %w", err)
	}
	return t, nil
}
