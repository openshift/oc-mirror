package v1

import (
	conditionsv1 "github.com/openshift/custom-resource-status/conditions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// UpdateServiceSpec defines the desired state of UpdateService.
type UpdateServiceSpec struct {
	// replicas is the number of pods to run. When >=2, a PodDisruptionBudget
	// will ensure that voluntary disruption leaves at least one Pod running at
	// all times.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Required
	Replicas int32 `json:"replicas"`

	// releases is the repository in which release images are tagged,
	// such as quay.io/openshift-release-dev/ocp-release.
	// +kubebuilder:validation:Required
	Releases string `json:"releases"`

	// graphDataImage is a container image that contains the UpdateService graph
	// data.
	// +kubebuilder:validation:Required
	GraphDataImage string `json:"graphDataImage"`
}

// UpdateServiceStatus defines the observed state of UpdateService.
type UpdateServiceStatus struct {
	// Conditions describe the state of the UpdateService resource.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +kubebuilder:validation:Optional
	Conditions []conditionsv1.Condition `json:"conditions,omitempty"  patchStrategy:"merge" patchMergeKey:"type"`

	// policyEngineURI is the external URI which exposes the policy
	// engine.  Available paths from this URI include:
	//
	// * /api/upgrades_info/v1/graph, with the update graph recommendations.
	PolicyEngineURI string `json:"policyEngineURI,optional"`
}

// Condition Types
const (
	// ConditionReconcileCompleted reports whether all required resources have been created
	// in the cluster and reflect the specified state.
	ConditionReconcileCompleted conditionsv1.ConditionType = "ReconcileCompleted"

	// ConditionRegistryCACertFound reports whether the updateservice registry CA cert had been found
	ConditionRegistryCACertFound conditionsv1.ConditionType = "RegistryCACertFound"

	ConditionReconcileError conditionsv1.ConditionType = "ReconcileError"
)

// +kubebuilder:object:root=true

// +kubebuilder:subresource:status
// +kubebuilder:resource:path=updateservices,scope=Namespaced

// UpdateService is the Schema for the updateservices API.
type UpdateService struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is standard object metadata.  More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +kubebuilder:validation:Required
	metav1.ObjectMeta `json:"metadata"`

	// spec is the desired state of the UpdateService service.  The
	// operator will work to ensure that the desired configuration is
	// applied to the cluster.
	// +kubebuilder:validation:Required
	Spec UpdateServiceSpec `json:"spec"`

	// status contains information about the current state of the
	// UpdateService service.
	// +kubebuilder:validation:Optional
	Status UpdateServiceStatus `json:"status"`
}

// +kubebuilder:object:root=true

// UpdateServiceList contains a list of UpdateService.
type UpdateServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UpdateService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&UpdateService{}, &UpdateServiceList{})
}
