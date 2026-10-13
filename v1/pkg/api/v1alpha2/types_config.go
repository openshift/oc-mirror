package v1alpha2

import (
	"path"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ImageSetConfiguration object kind.
	ImageSetConfigurationKind = "ImageSetConfiguration"
	OCITransportPrefix        = "oci:"
)

// ImageSetConfiguration configures image set creation.
type ImageSetConfiguration struct {
	metav1.TypeMeta `json:",inline"`
	// ImageSetConfigurationSpec defines the global configuration for an imageset.
	ImageSetConfigurationSpec `json:",inline"`
}

// ImageSetConfigurationSpec defines the global configuration for an imageset.
type ImageSetConfigurationSpec struct {
	// Mirror defines the configuration for content types within the imageset.
	Mirror Mirror `json:"mirror"`
	// ArchiveSize is the size of the segmented archive in GB
	ArchiveSize int64 `json:"archiveSize,omitempty"`
	// StorageConfig for reading/writing metadata and files.
	StorageConfig StorageConfig `json:"storageConfig"`
}

// Mirror defines the configuration for content types within the imageset.
type Mirror struct {
	// Platform defines the configuration for OpenShift and OKD platform types.
	Platform Platform `json:"platform,omitempty"`
	// Operators defines the configuration for Operator content types.
	Operators []Operator `json:"operators,omitempty"`
	// AdditionalImages defines the configuration for a list
	// of individual image content types.
	AdditionalImages []Image `json:"additionalImages,omitempty"`
	// Helm define the configuration for Helm content types.
	Helm Helm `json:"helm,omitempty"`
	// BlockedImages define a list of images that will be blocked
	// from the mirroring process if they exist in other content
	// types in the configuration.
	BlockedImages []Image `json:"blockedImages,omitempty"`
	// Samples defines the configuration for Sample content types.
	// This is currently not implemented.
	Samples []SampleImages `json:"samples,omitempty"`
}

// Platform defines the configuration for OpenShift and OKD platform types.
type Platform struct {
	// Graph defines whether Cincinnati graph data will
	// downloaded and publish
	Graph bool `json:"graph,omitempty"`
	// Channels defines the configuration for individual
	// OCP and OKD channels
	Channels []ReleaseChannel `json:"channels,omitempty"`
	// Architectures defines one or more architectures
	// to mirror for the release image. This is defined at the
	// platform level to enable cross-channel upgrades.
	Architectures []string `json:"architectures,omitempty"`
}

// ReleaseChannel defines the configuration for individual
// OCP and OKD channels
type ReleaseChannel struct {
	Name string `json:"name"`
	// Type of the platform in the context of this tool.
	// See the PlatformType enum for options. OCP is the default.
	Type PlatformType `json:"type"`
	// MinVersion is minimum version in the
	// release channel to mirror
	MinVersion string `json:"minVersion,omitempty"`
	// MaxVersion is maximum version in the
	// release channel to mirror
	MaxVersion string `json:"maxVersion,omitempty"`
	// ShortestPath mode calculates the shortest path
	// between the min and mav version
	ShortestPath bool `json:"shortestPath,omitempty"`
	// Full mode set the MinVersion to the
	// first release in the channel and the MaxVersion
	// to the last release in the channel.
	Full bool `json:"full,omitempty"`
}

// IsHeadsOnly determine if the mode set mirrors only channel head.
// Setting MaxVersion will override this setting.
func (r ReleaseChannel) IsHeadsOnly() bool {
	return !r.Full
}

// Operator defines the configuration for operator catalog mirroring.
type Operator struct {
	// Mirror specific operator packages, channels, and versions, and their dependencies.
	// If HeadsOnly is true, these objects are mirrored on top of heads of all channels.
	// Otherwise, only these specific objects are mirrored.
	IncludeConfig `json:",inline"`
	// Catalog image to mirror. This image must be pullable and available for subsequent
	// pulls on later mirrors.
	// This image should be an exact image pin (registry/namespace/name@sha256:<hash>)
	// but is not required to be.
	Catalog string `json:"catalog"`
	// TargetName is the target image name the catalog will be built with. If unset,
	// the catalog will be published with the provided name in the Catalog
	// field.
	// Deprecated in oc-mirror 4.13, to be replaced with TargetCatalog.
	TargetName string `json:"targetName,omitempty"`
	// TargetCatalog replaces TargetName and allows for specifying the exact URL of the target
	// catalog, including any path-components (organization, namespace) of the target catalog's location
	// on the disconnected registry.
	// This answer some customers requests regarding restrictions on where images can be placed.
	// The targetCatalog field consists of an optional namespace followed by the target image name,
	// described in extended Backus–Naur form below:
	//     target-catalog = [namespace '/'] target-name
	//     target-name    = path-component
	//     namespace      = path-component ['/' path-component]*
	//     path-component = alpha-numeric [separator alpha-numeric]*
	//     alpha-numeric  = /[a-z0-9]+/
	//     separator      = /[_.]|__|[-]*/
	// TargetCatalog will be preferred over TargetName if both are specified in te ImageSetConfig.
	TargetCatalog string `json:"targetCatalog,omitempty"`
	// TargetTag is the tag the catalog image will be built with. If unset,
	// the catalog will be publish with the provided tag in the Catalog
	// field or a tag calculated from the partial digest.
	TargetTag string `json:"targetTag,omitempty"`
	// Full defines whether all packages within the catalog
	// or specified IncludeConfig will be mirrored or just channel heads.
	Full bool `json:"full,omitempty"`
	// SkipDependencies will not include dependencies
	// of bundles included in the diff if true.
	SkipDependencies bool `json:"skipDependencies,omitempty"`
	// OriginalRef is used when the Catalog is an OCI FBC (File Based Catalog) location.
	// It contains the reference to the original repo on a remote registry
	// Deprecated in oc-mirror 4.13, and will no longer be used.
	OriginalRef string `json:"originalRef,omitempty"`
}

// GetUniqueName determines the catalog name that will
// be tracked in the metadata and built. This depends on what fields
// are set between Catalog, TargetCatalog (and soon deprecated
// TargetName), and TargetTag.
func (o Operator) GetUniqueName() (string, error) {
	ctlgRef := o.Catalog
	if o.TargetCatalog == "" && o.TargetName == "" && o.TargetTag == "" {
		return TrimProtocol(ctlgRef), nil
	}

	reg, ns, name, tag, id := ParseImageReference(ctlgRef)

	if o.TargetTag != "" {
		tag = o.TargetTag
		id = ""
	}
	uniqueName := ""

	if o.TargetCatalog != "" {
		// TargetCatalog takes precedence over TargetName, and replaces the catalog component-paths (URL)
		if !o.IsFBCOCI() && reg != "" {
			// reg is included in the name only in case of registry based catalogs.
			// the parsed reg is not relevant in case of OCI, because the parsed ref is simply a filesystem path here
			uniqueName += reg
		}
		uniqueName = path.Join(uniqueName, o.TargetCatalog)
	} else {
		uniqueName = path.Join(uniqueName, reg, ns)

		if o.TargetName != "" {
			name = o.TargetName
		}
		uniqueName = path.Join(uniqueName, name)
	}
	if tag != "" {
		uniqueName = uniqueName + ":" + tag
	} else {
		if id != "" {
			uniqueName = uniqueName + "@sha256:" + id
		}
	}
	return uniqueName, nil
}

// parseImageName returns the registry, organisation, repository, tag and digest
// from the imageName.
// It can handle both remote and local images.
func ParseImageReference(imageName string) (string, string, string, string, string) {
	registry, namespace, repo, tag, sha := "", "", "", "", ""
	imageName = TrimProtocol(imageName)
	imageName = strings.TrimPrefix(imageName, "/")
	imageName = strings.TrimSuffix(imageName, "/")
	pathComponents := strings.Split(imageName, "/")

	// For more than 2 pathComponents, the first must be the registry
	if len(pathComponents) > 1 {
		registry = pathComponents[0]
	}
	// For more than 3 pathComponents, everything in between registry (first) and
	// repository (last) is considered to be namespace or organisation
	if len(pathComponents) > 2 {
		namespace = strings.Join(pathComponents[1:len(pathComponents)-1], "/")
	}

	// It is best to split first on digest, as the `:` might
	// exist for the tag or for the digest
	img := strings.Split(pathComponents[len(pathComponents)-1], "@sha256:")
	// The first element in the slice will always exist (repository)
	repo = img[0]
	if len(img) > 1 {
		sha = img[1]
	}

	// We check now for the existance of a tag
	if strings.Contains(repo, ":") {
		nm := strings.Split(repo, ":")
		repo = nm[0]
		tag = nm[1]
	}

	return registry, namespace, repo, tag, sha
}

// trimProtocol removes oci://, file:// or docker:// from
// the parameter imageName
func TrimProtocol(imageName string) string {
	imageName = strings.TrimPrefix(imageName, OCITransportPrefix)
	imageName = strings.TrimPrefix(imageName, "file:")
	imageName = strings.TrimPrefix(imageName, "docker:")
	imageName = strings.TrimPrefix(imageName, "//")

	return imageName
}

// IsHeadsOnly determine if the mode set mirrors only channel heads of all packages in the catalog.
// Channels specified in DiffIncludeConfig will override this setting;
// heads will still be included, but prior versions may also be included.
func (o Operator) IsHeadsOnly() bool {
	return !o.Full
}

func (o Operator) IsFBCOCI() bool {
	return strings.HasPrefix(o.Catalog, OCITransportPrefix)
}

// Helm defines the configuration for Helm chart download
// and image mirroring
type Helm struct {
	// Repositories are the Helm repositories containing the charts
	Repositories []Repository `json:"repositories,omitempty"`
	// Local is the configuration for locally stored helm charts
	Local []Chart `json:"local,omitempty"`
}

// Repository defines the configuration for a Helm repository.
type Repository struct {
	// URL is the url of the Helm repository
	URL string `json:"url"`
	// Name is the name of the Helm repository
	Name string `json:"name"`
	// Charts is a list of charts to pull from the repo
	Charts []Chart `json:"charts"`
}

// Chart is the information an individual Helm chart
type Chart struct {
	// Chart is the chart name as define
	// in the Chart.yaml or in the Helm repo.
	Name string `json:"name"`
	// Version is the chart version as define in the
	// Chart.yaml or in the Helm repo.
	Version string `json:"version,omitempty"`
	// Path defines the path on disk where the
	// chart is stored.
	// This is applicable for a local chart.
	Path string `json:"path,omitempty"`
	// ImagePaths are custom JSON paths for images location
	// in the helm manifest or templates
	ImagePaths []string `json:"imagePaths,omitempty"`
}

// Image contains image pull information.
type Image struct {
	// Name of the image. This should be an exact image pin (registry/namespace/name@sha256:<hash>)
	// but is not required to be.
	Name string `json:"name"`
}

// SampleImages define the configuration
// for Sameple content types.
// Not implemented.
type SampleImages struct {
	Image `json:",inline"`
}
