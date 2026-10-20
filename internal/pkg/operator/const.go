package operator

const (
	operatorImageExtractDir           = "hold-operator" //TODO ALEX REMOVE ME when filtered_collector.go is the default
	operatorCatalogsDir        string = "operator-catalogs"
	operatorCatalogConfigDir   string = "catalog-config"
	operatorCatalogImageDir    string = "catalog-image"
	operatorCatalogFilteredDir string = "filtered-catalogs"
	operatorBundlesDir         string = "operator-bundles"
	operatorBundleImageDir     string = "bundle-image"
	operatorBundleManifestsDir string = "bundle-manifests"
	blobsDir                          = "blobs/sha256"
	collectorPrefix                   = "[OperatorImageCollector] "
	errMsg                            = collectorPrefix + "%s"
	errorSemver                string = " semver %v "
)
