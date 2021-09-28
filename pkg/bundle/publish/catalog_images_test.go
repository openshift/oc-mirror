package publish

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/stretchr/testify/require"
)

func Test_buildCatalogImage(t *testing.T) {

	ctx := context.TODO()
	ref := reference.DockerImageReference{
		Registry:  "localhost",
		Namespace: "test",
		Name:      "testname",
		Tag:       "vtest",
	}

	dir, _ := ioutil.TempDir("dir", "prefix")

	defer os.RemoveAll(dir)

	digest, can, err := buildCatalogImage(ctx, ref, dir)
	require.NoError(t, err)
	t.Log(digest)
	t.Log(can)

}
