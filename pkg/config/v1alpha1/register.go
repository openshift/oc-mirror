package v1alpha1

import "k8s.io/apimachinery/pkg/runtime/schema"

const (
	version = "v1alpha1"
	// TODO: what should this be?
	group = "mirror.openshift.io"
)

var (
	GroupVersion = schema.GroupVersion{Group: group, Version: version}
)
