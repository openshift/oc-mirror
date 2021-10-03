package publish

import (
	"context"
	"testing"

	"github.com/containers/image/v5/types"
	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/stretchr/testify/require"
)

func Test_buildCatalogImage(t *testing.T) {

	ctx := context.Background()

	dctx := types.SystemContext{
		DockerInsecureSkipTLSVerify: types.NewOptionalBool(true),
	}
	ref := reference.DockerImageReference{
		Registry:  "localhost:5000",
		Namespace: "test",
		Name:      "testname",
		Tag:       "vtest2",
	}

	dir := t.TempDir()
	//dir := "."

	can, err := buildCatalogImage(ctx, ref, dctx, dir)
	require.NoError(t, err)
	t.Log(err)
	t.Log(can)

}
