package cli

const (
	collecAllPrefix               string = "[CollectAll] "
	dockerProtocol                string = "docker://"
	ociProtocol                   string = "oci://"
	dirProtocol                   string = "dir://"
	fileProtocol                  string = "file://"
	releaseImageDir               string = "release-images"
	logsDir                       string = "logs"
	workingDir                    string = "working-dir"
	ocmirrorRelativePath          string = ".oc-mirror"
	cacheRelativePath             string = ".oc-mirror/.cache"
	cacheEnvVar                   string = "OC_MIRROR_CACHE"
	additionalImages              string = "additional-images"
	releaseImageExtractDir        string = "hold-release"
	cincinnatiGraphDataDir        string = "cincinnati-graph-data"
	operatorImageExtractDir       string = "hold-operator"
	operatorCatalogsDir           string = "operator-catalogs"
	signaturesDir                 string = "signatures"
	registryLogFilename           string = "registry.log"
	startMessage                  string = "starting local storage on localhost:%v"
	dryRunOutDir                  string = "dry-run"
	mappingFile                   string = "mapping.txt"
	missingImgsFile               string = "missing.txt"
	clusterResourcesDir           string = "cluster-resources"
	helmDir                       string = "helm"
	helmChartDir                  string = "charts"
	helmIndexesDir                string = "indexes"
	maxParallelLayerDownloads     uint   = 10
	maxParallelImageDownloads     uint   = 8
	limitOverallParallelDownloads uint   = 200
)
