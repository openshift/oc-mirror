package v2alpha1

import "k8s.io/apimachinery/pkg/runtime/schema"

const (
	version = "v2alpha1"
	group   = "mirror.openshift.io"
)

var (
	GroupVersion = schema.GroupVersion{Group: group, Version: version}
)
