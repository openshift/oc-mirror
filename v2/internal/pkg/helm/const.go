package helm

const (
	helmDir         string = "helm"
	helmChartDir    string = "charts"
	helmIndexesDir  string = "indexes"
	helmIndexFile   string = "index.yaml"
	dockerProtocol  string = "docker://"
	collectorPrefix string = "[HelmImageCollector] "
	errMsg          string = collectorPrefix + "%s"
)
