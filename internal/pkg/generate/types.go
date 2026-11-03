// Package generate reverse-engineers ImageSetConfiguration and
// DeleteImageSetConfiguration files from the content of a mirror registry.
package generate

import (
	"context"

	"github.com/operator-framework/operator-registry/alpha/model"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
)

// Source reads the content of a registry. Repository names are relative to
// the registry host (for example "openshift/release-images").
type Source interface {
	// Repositories lists the repositories to inspect.
	Repositories(ctx context.Context) ([]string, error)
	// Tags lists the tags of repo.
	Tags(ctx context.Context, repo string) ([]string, error)
	// IsCatalog reports whether repo:tag is an operator catalog image.
	IsCatalog(ctx context.Context, repo, tag string) (bool, error)
	// CatalogModel loads the declarative config of the catalog image repo:tag.
	CatalogModel(ctx context.Context, repo, tag string) (model.Model, error)
}

// Content is what was found in a registry, independent of the output kind.
type Content struct {
	Platform         v2alpha1.Platform
	Operators        []v2alpha1.Operator
	AdditionalImages []v2alpha1.AdditionalImage
	// Warnings describe lossy inference the user should review.
	Warnings []string
}
