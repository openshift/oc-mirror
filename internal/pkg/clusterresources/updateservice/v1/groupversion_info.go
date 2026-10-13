// Copied from https://github.com/openshift/cincinnati-operator/tree/master/api/v1
// The last release v1.0.3 doesn't even contain the API
// Commit : 425be2fc0ec501bfcf9bdb15f75df9d8a7b5ba6c
package v1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: "updateservice.operator.openshift.io", Version: "v1"}
)
