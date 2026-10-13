package cli

const (
	collecAllPrefix           string = "[CollectAll] "
	releaseImageDir           string = "release-images"
	logsDir                   string = "logs"
	workingDir                string = "working-dir"
	cacheRelativePath         string = ".oc-mirror/.cache"
	cacheEnvVar               string = "OC_MIRROR_CACHE"
	releaseImageExtractDir    string = "hold-release"
	cincinnatiGraphDataDir    string = "cincinnati-graph-data"
	operatorCatalogsDir       string = "operator-catalogs"
	signaturesDir             string = "signatures"
	startMessage              string = "starting local storage on localhost:%v"
	dryRunOutDir              string = "dry-run"
	mappingFile               string = "mapping.txt"
	missingImgsFile           string = "missing.txt"
	clusterResourcesDir       string = "cluster-resources"
	helmDir                   string = "helm"
	helmChartDir              string = "charts"
	helmIndexesDir            string = "indexes"
	maxParallelLayerDownloads uint   = 5
	maxParallelImageDownloads uint   = 4
)
