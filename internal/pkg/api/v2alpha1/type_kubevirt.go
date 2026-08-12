package v2alpha1

import (
	"time"
)

// InstallerBootableImages
// layout does not use the github.com/hypershift/support/releaseinfo
// api's directly as it is a large overhead using
// the k8s.io/apimachinery dependencies just to extract the kubevirt container
type InstallerBootableImages struct {
	Stream        string        `json:"stream"`
	Metadata      Metadata      `json:"metadata"`
	Architectures Architectures `json:"architectures"`
}

type Metadata struct {
	LastModified time.Time `json:"last-modified"`
	Generator    string    `json:"generator"`
}

type Artifacts struct {
	Kubevirt Kubevirt `json:"kubevirt"`
}

type Kubevirt struct {
	Release   string `json:"release"`
	Image     string `json:"image"`
	DigestRef string `json:"digest-ref"`
}

type Images struct {
	Kubevirt Kubevirt `json:"kubevirt"`
}

// ArchImages holds the bootable image data for a single CPU architecture.
// The JSON key names (x86_64, aarch64, etc.) come directly from the
// 0000_50_installer_coreos-bootimages ConfigMap embedded in each release payload.
type ArchImages struct {
	Artifacts Artifacts `json:"artifacts"`
	Images    Images    `json:"images"`
}

// X86_64 is kept as a type alias for ArchImages to preserve backward
// compatibility with code that was written before multi-arch support.
type X86_64 = ArchImages

type Architectures struct {
	X86_64  ArchImages `json:"x86_64"`
	Aarch64 ArchImages `json:"aarch64"`
	Ppc64le ArchImages `json:"ppc64le"`
	S390x   ArchImages `json:"s390x"`
}

// InstallerConfigMap - this is the yaml structure
// in the form of a configmap that hold the json formatted
// structure of interest in the Data.Stream field
type InstallerConfigMap struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Annotations struct {
			IncludeReleaseOpenshiftIoIbmCloudManaged             string `yaml:"include.release.openshift.io/ibm-cloud-managed"`
			IncludeReleaseOpenshiftIoSelfManagedHighAvailability string `yaml:"include.release.openshift.io/self-managed-high-availability"`
			IncludeReleaseOpenshiftIoSingleNodeDeveloper         string `yaml:"include.release.openshift.io/single-node-developer"`
		} `yaml:"annotations"`
		CreationTimestamp interface{} `yaml:"creationTimestamp"`
		Name              string      `yaml:"name"`
		Namespace         string      `yaml:"namespace"`
	} `yaml:"metadata"`
	APIVersion string `yaml:"apiVersion"`
	Data       struct {
		ReleaseVersion string `yaml:"releaseVersion"`
		Stream         string `yaml:"stream"`
	} `yaml:"data"`
}
