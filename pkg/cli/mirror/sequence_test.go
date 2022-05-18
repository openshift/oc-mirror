package mirror

import (
	"errors"
	"os"
	"testing"

	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/metadata/storage"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestCheckSequence(t *testing.T) {
	type spec struct {
		name       string
		opts       *MirrorOptions
		incoming   v1alpha2.Metadata
		current    v1alpha2.Metadata
		backendErr error
		expErr     error
	}

	tests := []spec{
		{
			name: "Invalid/FirstRun",
			opts: &MirrorOptions{
				RootOptions: &cli.RootOptions{
					IOStreams: genericclioptions.IOStreams{
						In:     os.Stdin,
						Out:    os.Stdout,
						ErrOut: os.Stderr,
					},
				},
			},
			incoming: v1alpha2.Metadata{
				MetadataSpec: v1alpha2.MetadataSpec{
					PastMirror: v1alpha2.PastMirror{
						Sequence: 2,
					},
				},
			},
			backendErr: storage.ErrMetadataNotExist,
			expErr:     &ErrInvalidSequence{1, 2},
		},
		{
			name: "Invalid/OutOfOrder",
			opts: &MirrorOptions{
				RootOptions: &cli.RootOptions{
					IOStreams: genericclioptions.IOStreams{
						In:     os.Stdin,
						Out:    os.Stdout,
						ErrOut: os.Stderr,
					},
				},
			},
			incoming: v1alpha2.Metadata{
				MetadataSpec: v1alpha2.MetadataSpec{
					PastMirror: v1alpha2.PastMirror{
						Sequence: 3,
					},
				},
			},
			current: v1alpha2.Metadata{
				MetadataSpec: v1alpha2.MetadataSpec{
					PastMirror: v1alpha2.PastMirror{
						Sequence: 1,
					},
				},
			},
			expErr: &ErrInvalidSequence{2, 3},
		},
		{
			name: "Invalid/UndefinedError",
			opts: &MirrorOptions{
				RootOptions: &cli.RootOptions{
					IOStreams: genericclioptions.IOStreams{
						In:     os.Stdin,
						Out:    os.Stdout,
						ErrOut: os.Stderr,
					},
				},
				DestSkipTLS: true,
			},
			backendErr: errors.New("some error"),
			expErr:     errors.New("some error"),
		},
		{
			name: "Valid/SkipMetadataCheck",
			opts: &MirrorOptions{
				RootOptions: &cli.RootOptions{
					IOStreams: genericclioptions.IOStreams{
						In:     os.Stdin,
						Out:    os.Stdout,
						ErrOut: os.Stderr,
					},
				},
				SkipMetadataCheck: true,
			},
			incoming: v1alpha2.Metadata{
				MetadataSpec: v1alpha2.MetadataSpec{
					PastMirror: v1alpha2.PastMirror{
						Sequence: 2,
					},
				},
			},
			current: v1alpha2.Metadata{
				MetadataSpec: v1alpha2.MetadataSpec{
					PastMirror: v1alpha2.PastMirror{
						Sequence: 2,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.checkSequence(tt.incoming, tt.current, tt.backendErr)
			if tt.expErr == nil {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.expErr.Error())
			}
		})
	}
}
