package registriesd

type registryConfiguration struct {
	// The key is a namespace, using fully-expanded Docker reference format or parent namespaces (per dockerReference.PolicyConfiguration*),
	Docker map[string]registryNamespace `yaml:"docker"`
}

// registryNamespace defines lookaside locations for a single namespace.
type registryNamespace struct {
	Lookaside              string `yaml:"lookaside"`                // For reading, and if LookasideStaging is not present, for writing.
	LookasideStaging       string `yaml:"lookaside-staging"`        // For writing only.
	SigStore               string `yaml:"sigstore"`                 // For compatibility, deprecated in favor of Lookaside.
	SigStoreStaging        string `yaml:"sigstore-staging"`         // For compatibility, deprecated in favor of LookasideStaging.
	UseSigstoreAttachments bool   `yaml:"use-sigstore-attachments"` // originally , this had omitempty, so the type was *bool
}
