package v1alpha2

// StorageConfig configures how metadata is stored.
type StorageConfig struct {
	// Registry defines the configuration for registry
	// storage types.
	Registry *RegistryConfig `json:"registry,omitempty"`
	// Local defines the configuration for local
	// storage types.
	Local *LocalConfig `json:"local,omitempty"`
}

// RegistryConfig configures a registry-based storage.
type RegistryConfig struct {
	// ImageURL at which the image can be pulled.
	ImageURL string `json:"imageURL"`
	// SkipTLS defines whether to use TLS validation
	// when interacting the the defined registry.
	SkipTLS bool `json:"skipTLS"`
}

// LocalConfig configure a local directory storage
type LocalConfig struct {
	Path string `json:"path"`
}

// IsSet will determine whether StorageConfig
// is empty or has backends set
func (s StorageConfig) IsSet() bool {
	if s.Registry != nil || s.Local != nil {
		return true
	}
	return false
}
