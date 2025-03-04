package helm

import (
	"context"
	"net/http"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

//go:generate mockgen -source=./interface.go -destination=./mock/interface_generated.go -package=mock

type CollectorInterface interface {
	HelmImageCollector(ctx context.Context) ([]v2alpha1.CopyImageSchema, error)
}

type indexDownloader interface {
	DownloadIndexFile() (string, error)
}

type chartDownloader interface {
	DownloadTo(ref, version, dest string) (string, any, error)
}

type webClient interface {
	Get(url string) (resp *http.Response, err error)
}
