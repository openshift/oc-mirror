package operator

const (
	operatorImageExtractDir = "hold-operator"
	dockerProtocol          = "docker://"
	ociProtocol             = "oci://"
	ociProtocolTrimmed      = "oci:"
	operatorImageDir        = "operator-images"
	blobsDir                = "blobs/sha256"
	collectorPrefix         = "[OperatorImageCollector] "
	errMsg                  = collectorPrefix + "%s"
	logsFile                = "operator.log"
)
