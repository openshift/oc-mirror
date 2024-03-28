package v1alpha3

import (
	"time"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha2"
	"github.com/operator-framework/operator-registry/alpha/property"
)

// ReleaseSchema
type ReleaseSchema struct {
	Kind       string   `json:"kind"`
	APIVersion string   `json:"apiVersion"`
	Metadata   Metadata `json:"metadata"`
	Spec       Spec     `json:"spec"`
	Status     Status   `json:"status"`
}

// MetadataAnnotations
type MetadataAnnotations struct {
	ReleaseOpenshiftIoFromImageStream string `json:"release.openshift.io/from-image-stream"`
	ReleaseOpenshiftIoFromRelease     string `json:"release.openshift.io/from-release"`
}

// Metadata
type Metadata struct {
	Name              string              `json:"name"`
	CreationTimestamp time.Time           `json:"creationTimestamp"`
	Annotations       MetadataAnnotations `json:"annotations"`
}

// LookupPolicy
type LookupPolicy struct {
	Local bool `json:"local"`
}

// Annotations
type Annotations struct {
	IoOpenshiftBuildCommitID            string `json:"io.openshift.build.commit.id"`
	IoOpenshiftBuildCommitRef           string `json:"io.openshift.build.commit.ref"`
	IoOpenshiftBuildSourceLocation      string `json:"io.openshift.build.source-location"`
	IoOpenshiftBuildVersionDisplayNames string `json:"io.openshift.build.version-display-names"`
	IoOpenshiftBuildVersions            string `json:"io.openshift.build.versions"`
}

// From
type From struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// ImportPolicy
type ImportPolicy struct {
}

// ReferencePolicy
type ReferencePolicy struct {
	Type string `json:"type"`
}

// Tags
type Tags struct {
	Name            string          `json:"name"`
	Annotations     Annotations     `json:"annotations,omitempty"`
	From            From            `json:"from"`
	Generation      interface{}     `json:"generation"`
	ImportPolicy    ImportPolicy    `json:"importPolicy"`
	ReferencePolicy ReferencePolicy `json:"referencePolicy"`
}

// Spec
type Spec struct {
	LookupPolicy LookupPolicy `json:"lookupPolicy"`
	Tags         []Tags       `json:"tags"`
}

// Status
type Status struct {
	DockerImageRepository string `json:"dockerImageRepository"`
}

// OCISchema
type OCISchema struct {
	SchemaVersion int           `json:"schemaVersion"`
	MediaType     string        `json:"mediaType,omitempty"`
	Manifests     []OCIManifest `json:"manifests"`
	Config        OCIManifest   `json:"config,omitempty"`
	Layers        []OCIManifest `json:"layers,omitempty"`
}

type OCIManifest struct {
	MediaType string `json:"mediaType,omitempty"`
	Digest    string `json:"digest,omitempty"`
	Size      int    `json:"size,omitempty"`
}

// OperatorConfigSchema
type OperatorConfigSchema struct {
	Created      time.Time         `json:"created"`
	Architecture string            `json:"architecture"`
	Os           string            `json:"os"`
	Config       OperatorConfig    `json:"config"`
	RootFS       OperatorRootFS    `json:"rootfs"`
	History      []OperatorHistory `json:"history"`
}

type OperatorConfig struct {
	User         string `json:"User"`
	ExposedPorts struct {
		TCP struct {
		} `json:"tcp"`
	} `json:"ExposedPorts"`
	Env        []string       `json:"Env"`
	Entrypoint []string       `json:"Entrypoint"`
	Cmd        []string       `json:"Cmd"`
	WorkingDir string         `json:"WorkingDir"`
	Labels     OperatorLabels `json:"Labels"`
}

type OperatorLabels struct {
	License                                    string `json:"License"`
	Architecture                               string `json:"architecture"`
	BuildDate                                  string `json:"build-date"`
	ComRedhatBuildHost                         string `json:"com.redhat.build-host"`
	ComRedhatComponent                         string `json:"com.redhat.component"`
	ComRedhatIndexDeliveryDistributionScope    string `json:"com.redhat.index.delivery.distribution_scope"`
	ComRedhatIndexDeliveryVersion              string `json:"com.redhat.index.delivery.version"`
	ComRedhatLicenseTerms                      string `json:"com.redhat.license_terms"`
	Description                                string `json:"description"`
	DistributionScope                          string `json:"distribution-scope"`
	IoBuildahVersion                           string `json:"io.buildah.version"`
	IoK8SDescription                           string `json:"io.k8s.description"`
	IoK8SDisplayName                           string `json:"io.k8s.display-name"`
	IoOpenshiftBuildCommitID                   string `json:"io.openshift.build.commit.id"`
	IoOpenshiftBuildCommitURL                  string `json:"io.openshift.build.commit.url"`
	IoOpenshiftBuildSourceLocation             string `json:"io.openshift.build.source-location"`
	IoOpenshiftExposeServices                  string `json:"io.openshift.expose-services"`
	IoOpenshiftMaintainerComponent             string `json:"io.openshift.maintainer.component"`
	IoOpenshiftMaintainerProduct               string `json:"io.openshift.maintainer.product"`
	IoOpenshiftMaintainerProject               string `json:"io.openshift.maintainer.project"`
	IoOpenshiftTags                            string `json:"io.openshift.tags"`
	Maintainer                                 string `json:"maintainer"`
	Name                                       string `json:"name"`
	OperatorsOperatorframeworkIoIndexConfigsV1 string `json:"operators.operatorframework.io.index.configs.v1"`
	Release                                    string `json:"release"`
	Summary                                    string `json:"summary"`
	URL                                        string `json:"url"`
	VcsRef                                     string `json:"vcs-ref"`
	VcsType                                    string `json:"vcs-type"`
	Vendor                                     string `json:"vendor"`
	Version                                    string `json:"version"`
}

type OperatorRootFS struct {
	Type    string   `json:"type"`
	DiffIds []string `json:"diff_ids"`
}

type OperatorHistory struct {
	Created    time.Time `json:"created"`
	Comment    string    `json:"comment,omitempty"`
	CreatedBy  string    `json:"created_by,omitempty"`
	EmptyLayer bool      `json:"empty_layer,omitempty"`
}

// Package
type Package struct {
	Schema         string              `json:"schema"`
	Name           string              `json:"name"`
	DefaultChannel string              `json:"defaultChannel"`
	Icon           *Icon               `json:"icon,omitempty"`
	Description    string              `json:"description,omitempty"`
	Properties     []property.Property `json:"properties,omitempty" hash:"set"`
}

// Icon
type Icon struct {
	Data      []byte `json:"base64data"`
	MediaType string `json:"mediatype"`
}

// RelatedImage - used to index images
type RelatedImage struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	// Type: metadata to explain why this image is being copied
	// it doesn't need to be persisted to JSON
	// This field doesn't exist in the catalog declarativeConfig.
	Type v1alpha2.ImageType `json:"-"`
	// Used to keep specific tag info for the related image
	// if set should be used when mirroring
	TargetTag string `json:"targetTag"`
	// Used to keep specific naming info for the related image
	// if set should be used when mirroring
	TargetName string `json:"tagetName"`
}

// CopyImageSchema
type CopyImageSchema struct {
	// Source: from where to copy the image
	Source string
	// Destination: to where should the image be copied
	Destination string
	// Origin: Original reference to the image
	Origin string
	// Type: metadata to explain why this image is being copied
	// it doesn´t need to be persisted to json
	Type v1alpha2.ImageType `json:"-"`
}

// SignatureContentSchema
type SignatureContentSchema struct {
	Critical struct {
		Image struct {
			DockerManifestDigest string `json:"docker-manifest-digest"`
		} `json:"image"`
		Type     string `json:"type"`
		Identity struct {
			DockerReference string `json:"docker-reference"`
		} `json:"identity"`
	} `json:"critical"`
	Optional struct {
		Creator string `json:"creator"`
	} `json:"optional"`
}

// DeleteImageList
type DeleteImageList struct {
	Kind       string       `json:"kind"`
	APIVersion string       `json:"apiVersion"`
	Items      []DeleteItem `json:"items"`
}

type DeleteItem struct {
	ImageName      string   `json:"imageName"`
	ImageReference string   `json:"imageReference"`
	RelatedBlobs   []string `json:"relatedBlobs"`
}
