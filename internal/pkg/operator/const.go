package operator

const (
	operatorImageExtractDir           = "hold-operator" //TODO ALEX REMOVE ME when filtered_collector.go is the default
	dockerProtocol                    = "docker://"
	ociProtocol                       = "oci://"
	ociProtocolTrimmed                = "oci:"
	operatorImageDir                  = "operator-images" //TODO ALEX REMOVE ME when filtered_collector.go is the default
	operatorCatalogsDir        string = "operator-catalogs"
	operatorCatalogConfigDir   string = "catalog-config"
	operatorCatalogImageDir    string = "catalog-image"
	operatorCatalogFilteredDir string = "filtered-catalogs"
	blobsDir                          = "blobs/sha256"
	collectorPrefix                   = "[OperatorImageCollector] "
	errMsg                            = collectorPrefix + "%s"
	logsFile                          = "operator.log"
	errorSemver                string = " semver %v "
	filteredCatalogDir                = "filtered-operator"
	digestIncorrectMessage     string = "the digests seem to be incorrect for %s: %s "
)
