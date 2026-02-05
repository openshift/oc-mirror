package registriesd

type registryConfiguration struct {
	// The key is a namespace, using fully-expanded Docker reference format or parent namespaces (per dockerReference.PolicyConfiguration*),
	Docker        map[string]registryNamespace `json:"docker,omitempty"`
	DefaultDocker *registryNamespace           `json:"default-docker,omitempty"`
}

// registryNamespace defines lookaside locations for a single namespace.
type registryNamespace struct {
	Lookaside              string `json:"lookaside,omitempty"`                // For reading, and if LookasideStaging is not present, for writing.
	LookasideStaging       string `json:"lookaside-staging,omitempty"`        // For writing only.
	SigStore               string `json:"sigstore,omitempty"`                 // For compatibility, deprecated in favor of Lookaside.
	SigStoreStaging        string `json:"sigstore-staging,omitempty"`         // For compatibility, deprecated in favor of LookasideStaging.
	UseSigstoreAttachments *bool  `json:"use-sigstore-attachments,omitempty"` // Since the zero-value of bool is false, it is needed to have a pointer to identify when a field was not set.
}

func boolPtr(b bool) *bool {
	return &b
}
