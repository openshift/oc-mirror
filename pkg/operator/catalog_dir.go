package operator

import (
	"path/filepath"

	imgreference "github.com/openshift/library-go/pkg/image/reference"
)

// GenerateCatalogDir will generate a directory location for a catalog from a reference
func GenerateCatalogDir(ctlgRef imgreference.DockerImageReference) (string, error) {
	leafDir := ctlgRef.Tag
	if leafDir == "" {
		leafDir = ctlgRef.ID
	}
	if leafDir == "" {
		// return "", fmt.Errorf("catalog %q must have either a tag or digest", ctlgRef.Exact())
		return filepath.Join(ctlgRef.Registry, ctlgRef.Namespace, ctlgRef.Name), nil
	}
	return filepath.Join(ctlgRef.Registry, ctlgRef.Namespace, ctlgRef.Name, leafDir), nil
}
