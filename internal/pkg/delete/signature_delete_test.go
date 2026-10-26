package delete

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openshift/oc-mirror/v2/internal/pkg/api/v2alpha1"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

type signatureLookup struct {
	refs []string
	tags []string
	err  error
}

func (s *signatureLookup) GetSignatureTag(_ context.Context, ref string) ([]string, error) {
	s.refs = append(s.refs, ref)
	return s.tags, s.err
}

func TestReleaseSignatureDeletionWithoutCache(t *testing.T) {
	source := &signatureLookup{err: errors.New("not cached")}
	destination := &signatureLookup{tags: []string{"sha256-" + strings.Repeat("a", 64) + ".sig"}}
	d := DeleteImages{
		Opts:           mirror.CopyOptions{Global: &mirror.GlobalOptions{DeleteSignatures: true}},
		SigHandler:     source,
		DestSigHandler: destination,
	}
	img := v2alpha1.CopyImageSchema{
		Origin:      "quay.io/source/release:v1",
		Source:      "docker://localhost:55000/custom/release:v1",
		Destination: "docker://mirror.example.com/custom/release:v1",
		Type:        v2alpha1.TypeOCPRelease,
	}
	items := d.sigDeleteItems(context.Background(), img)
	require.Equal(t, []string{img.Source}, source.refs)
	require.Equal(t, []string{img.Destination}, destination.refs)
	require.Len(t, items, 1)
	require.Equal(t, "docker://mirror.example.com/custom/release:"+destination.tags[0], items[0].ImageReference)
}
