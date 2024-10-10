package helm

import (
	"path/filepath"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func New(log clog.PluggableLoggerInterface,
	config v2alpha1.ImageSetConfiguration,
	opts mirror.CopyOptions,
	indexDownloader indexDownloader,
	chartDownloader chartDownloader,
	httpClient webClient,
) CollectorInterface {
	lsc = &LocalStorageCollector{Log: log, Config: config, Opts: opts, Helm: NewHelmOptions(opts.SrcImage.TlsVerify)}
	lsc.Log.Debug("helm.New opts.SrcImage.TlsVerify %t", opts.SrcImage.TlsVerify)

	wClient = httpClient

	cleanup, file, _ := createTempFile(filepath.Join(lsc.Opts.Global.WorkingDir, helmDir))
	lsc.Helm.settings.RepositoryConfig = file
	lsc.cleanup = cleanup

	lsc.Downloaders.indexDownloader = indexDownloader

	if chartDownloader == nil {
		lsc.Downloaders.chartDownloader = GetDefaultChartDownloader()

	} else {
		lsc.Downloaders.chartDownloader = chartDownloader
	}

	return lsc
}
