package v1alpha3

import (
	"time"

	"github.com/operator-framework/operator-registry/alpha/property"
)

const (
	SchemaPackage = "olm.package"
	SchemaChannel = "olm.channel"
	SchemaBundle  = "olm.bundle"
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
	MediaType     string        `json:"mediaType"`
	Manifests     []OCIManifest `json:"manifests"`
	Config        OCIManifest   `json:"config"`
	Layers        []OCIManifest `json:"layers"`
}

type OCIManifest struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int    `json:"size"`
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

// Channel used in parsing channel data
type Channel struct {
	Schema     string              `json:"schema"`
	Name       string              `json:"name"`
	Package    string              `json:"package"`
	Entries    []ChannelEntry      `json:"entries"`
	Properties []property.Property `json:"properties,omitempty" hash:"set"`
}

// ChannelEntry used in the Channel struct
type ChannelEntry struct {
	Name      string   `json:"name"`
	Replaces  string   `json:"replaces,omitempty"`
	Skips     []string `json:"skips,omitempty"`
	SkipRange string   `json:"skipRange,omitempty"`
}

// Bundle specifies all metadata and data of a bundle object.
// Top-level fields are the source of truth, i.e. not CSV values.
//
// Notes:
//   - Any field slice type field or type containing a slice somewhere
//     where two types/fields are equal if their contents are equal regardless
//     of order must have a `hash:"set"` field tag for bundle comparison.
//   - Any fields that have a `json:"-"` tag must be included in the equality
//     evaluation in bundlesEqual().
type Bundle struct {
	Schema        string              `json:"schema"`
	Name          string              `json:"name"`
	Package       string              `json:"package"`
	Image         string              `json:"image"`
	Properties    []property.Property `json:"properties,omitempty" hash:"set"`
	RelatedImages []RelatedImage      `json:"relatedImages,omitempty" hash:"set"`

	// These fields are present so that we can continue serving
	// the GRPC API the way packageserver expects us to in a
	// backwards-compatible way. These are populated from
	// any `olm.bundle.object` properties.
	//
	// These fields will never be persisted in the bundle blob as
	// first class fields.
	CsvJSON string   `json:"-"`
	Objects []string `json:"-"`
}

// RelatedImage - used to index images
type RelatedImage struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

// DeclarativeConfig this updates the existing dclrcfg
type DeclarativeConfig struct {
	Schema         string              `json:"schema"`
	Name           string              `json:"name"`
	Package        string              `json:"package"`
	Properties     []property.Property `json:"properties,omitempty" hash:"set"`
	RelatedImages  []RelatedImage      `json:"relatedImages,omitempty" hash:"set"`
	Entries        []ChannelEntry      `json:"entries"`
	DefaultChannel string              `json:"defaultChannel"`
	Description    string              `json:"description,omitempty"`
}

// ISCPackage used in the map to check for
// min and max version
type ISCPackage struct {
	Channel    string
	MinVersion string
	MaxVersion string
	Full       bool
}

// ImageRefSchema used to return parsed Image data
type ImageRefSchema struct {
	Repository string
	Namespace  string
	Component  string
}

// CopyImageSchema
type CopyImageSchema struct {
	Source      string
	Destination string
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
