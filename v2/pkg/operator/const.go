package operator

const (
	indexJson               string = "index.json"
	operatorImageExtractDir string = "hold-operator"
	dockerProtocol          string = "docker://"
	ociProtocolTrimmed      string = "oci:"
	operatorImageDir        string = "operator-images"
	blobsDir                string = "blobs/sha256"
	diskToMirror            string = "diskToMirror"
	mirrorToDisk            string = "mirrorToDisk"
	errMsg                  string = "[OperatorImageCollector] %v "
	logsFile                string = "logs/operator.log"
)
