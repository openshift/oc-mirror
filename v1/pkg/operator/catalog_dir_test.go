package operator

import (
	"testing"

	imgreference "github.com/openshift/library-go/pkg/image/reference"
	"github.com/stretchr/testify/require"
)

func TestGenerateCatalogDir(t *testing.T) {

	exp := "registry.com/catalog/latest"
	ctlg := "registry.com/catalog:latest"
	ctlgRef, err := imgreference.Parse(ctlg)
	require.NoError(t, err)

	actual, err := GenerateCatalogDir(ctlgRef)
	require.NoError(t, err)

	require.Equal(t, exp, actual)
}
