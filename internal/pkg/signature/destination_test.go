package signature

import (
	"context"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/types"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	ocmmanifest "github.com/openshift/oc-mirror/v2/internal/pkg/manifest"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type destinationManifest struct {
	ocmmanifest.ManifestInterface
	systemContext *types.SystemContext
}

func (m *destinationManifest) ImageManifest(_ context.Context, sctx *types.SystemContext, _ string, _ *digest.Digest) ([]byte, string, error) {
	m.systemContext = sctx
	return []byte(`{"schemaVersion":2}`), manifest.DockerV2Schema2MediaType, nil
}

func TestDestinationSignatureLookupUsesDestinationOptions(t *testing.T) {
	for _, tc := range []struct {
		name      string
		verifyTLS bool
	}{
		{"TLS verification enabled", true},
		{"TLS verification disabled", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			global := &mirror.GlobalOptions{}
			_, shared := mirror.SharedImageFlags()
			_, deprecated := mirror.DeprecatedTLSVerifyFlags()
			flags, dest := mirror.ImageDestFlags(global, shared, deprecated, "dest-", "dcreds")
			require.NoError(t, flags.Set("dest-creds", "destination-user:destination-password"))
			dest.TlsVerify = tc.verifyTLS
			opts := &mirror.CopyOptions{Global: global, DestImage: dest}
			inspector := &destinationManifest{}
			handler := NewForDestination(opts, clog.New("error"))
			handler.ocmirrormanifest = inspector
			tags, err := handler.GetSignatureTag(context.Background(), "docker://mirror.example.com/custom/release:v1")
			require.NoError(t, err)
			require.Len(t, tags, 1)
			require.Equal(t, types.NewOptionalBool(!tc.verifyTLS), inspector.systemContext.DockerInsecureSkipTLSVerify)
			require.Equal(t, "destination-user", inspector.systemContext.DockerAuthConfig.Username)
			require.Equal(t, "destination-password", inspector.systemContext.DockerAuthConfig.Password)
		})
	}
}
