package v1alpha2

import "k8s.io/apimachinery/pkg/runtime/schema"

const (
	version = "v1alpha2"
	group   = "mirror.openshift.io"
)

var (
	GroupVersion = schema.GroupVersion{Group: group, Version: version}
)
