package clusterresources

const (
	clusterResourcesDir            string = "cluster-resources"
	updateServiceFilename          string = "updateService.yaml"
	updateServiceResourceName      string = "update-service-oc-mirror"
	updateServiceResourceKind      string = "UpdateService"
	configMapApiVersion                   = "v1"
	configMapKind                         = "ConfigMap"
	configMapBinaryDataIndexFormat        = "sha256-%s-%d"
	signatureNamespace                    = "openshift-config-managed"
	configMapName                         = "mirrored-release-signatures"
	signatureLabel                        = "release.openshift.io/verification-signatures"
	signatureConfigMapMsg                 = "[GenerateSignatureConfigMap] %v"
	signatureDir                          = "signatures"
)
