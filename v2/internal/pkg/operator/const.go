package operator

const (
	operatorImageExtractDir        = "hold-operator"
	dockerProtocol                 = "docker://"
	ociProtocol                    = "oci://"
	ociProtocolTrimmed             = "oci:"
	operatorImageDir               = "operator-images"
	blobsDir                       = "blobs/sha256"
	collectorPrefix                = "[OperatorImageCollector] "
	errMsg                         = collectorPrefix + "%s"
	logsFile                       = "operator.log"
	errorSemver             string = " semver %v "
	filteredCatalogDir             = "filtered-operator"
	digestIncorrectMessage  string = "the digests seem to be incorrect for %s: %s "
)
