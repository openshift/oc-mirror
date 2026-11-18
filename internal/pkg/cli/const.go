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
	// Upper bound for --parallel-images. Upstream caps it at 10 (OCPBUGS-58467),
	// but that bug was about 0 hanging, not about high values failing - its own
	// reproducer ran --parallel-images 100 to completion. The batch worker is a
	// plain semaphore, so this number is the real concurrency. 128 is a typo
	// guard, not a recommendation: the registries on either end decide where
	// more stops helping.
	parallelImagesCeiling uint = 128
)
