package v1alpha1

// StorageConfig configures how metadata is stored.
type StorageConfig struct {
	Registry *RegistryConfig `json:"registry,omitempty"`
	Local    *LocalConfig    `json:"local,omitempty"`
}

// RegistryConfig configures a registry-based storage.
type RegistryConfig struct {
	// ImageURL at which the image can be pulled.
	ImageURL string `json:"imageURL"`
	SkipTLS  bool   `json:"skipTLS"`
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
