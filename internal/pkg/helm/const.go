package helm

const (
	helmDir         string = "helm"
	helmChartDir    string = "charts"
	helmIndexesDir  string = "indexes"
	helmValuesDir   string = "values"
	helmIndexFile   string = "index.yaml"
	collectorPrefix string = "[HelmImageCollector] "
	errMsg          string = collectorPrefix + "%s"
)
