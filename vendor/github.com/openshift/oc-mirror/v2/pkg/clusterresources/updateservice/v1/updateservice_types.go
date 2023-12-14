// Copied from https://github.com/openshift/cincinnati-operator/tree/master/api/v1
// The last release v1.0.3 doesn't even contain the API
// Commit : 425be2fc0ec501bfcf9bdb15f75df9d8a7b5ba6c

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// UpdateServiceSpec defines the desired state of UpdateService.
type UpdateServiceSpec struct {
	// replicas is the number of pods to run. When >=2, a PodDisruptionBudget
	// will ensure that voluntary disruption leaves at least one Pod running at
	// all times.
	Replicas int32 `json:"replicas"`

	// releases is the repository in which release images are tagged,
	// such as quay.io/openshift-release-dev/ocp-release.
	Releases string `json:"releases"`

	// graphDataImage is a container image that contains the UpdateService graph
	// data.
	GraphDataImage string `json:"graphDataImage"`
}

// UpdateServiceStatus defines the observed state of UpdateService.
type UpdateServiceStatus struct{}

// UpdateService is the Schema for the updateservices API.
type UpdateService struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is standard object metadata.  More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	metav1.ObjectMeta `json:"metadata"`

	// spec is the desired state of the UpdateService service.  The
	// operator will work to ensure that the desired configuration is
	// applied to the cluster.
	Spec UpdateServiceSpec `json:"spec"`

	// status contains information about the current state of the
	// UpdateService service.
	Status UpdateServiceStatus `json:"status"`
}
