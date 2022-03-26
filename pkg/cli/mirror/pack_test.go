package mirror

import (
	"context"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/config"
	"github.com/openshift/oc-mirror/pkg/config/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/stretchr/testify/require"
)

func TestPack(t *testing.T) {
	type spec struct {
		desc    string
		opts    *MirrorOptions
		meta    v1alpha2.Metadata
		assocs  image.AssociationSet
		updates bool
	}

	cases := []spec{
		{
			desc: "Success/NoUpdates",
			opts: &MirrorOptions{
				RootOptions:     &cli.RootOptions{},
				ContinueOnError: false,
				SkipMissing:     false,
			},
			assocs: image.AssociationSet{"imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": map[string]image.Association{
				"imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": {
					Name:            "imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Path:            "single_manifest",
					TagSymlink:      "latest",
					ID:              "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Type:            image.TypeGeneric,
					ManifestDigests: nil,
					LayerDigests: []string{
						"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
						"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
						"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
						"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
						"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
						"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
					},
				},
			},
			},
			meta: v1alpha2.Metadata{
				MetadataSpec: v1alpha2.MetadataSpec{
					PastAssociations: []image.Association{
						{
							Name:            "imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
							Path:            "single_manifest",
							TagSymlink:      "latest",
							ID:              "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
							Type:            image.TypeGeneric,
							ManifestDigests: nil,
							LayerDigests: []string{
								"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
								"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
								"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
								"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
								"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
								"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
							},
						},
					},
				},
			},
		},
		{
			desc: "Success/Updates",
			opts: &MirrorOptions{
				RootOptions:     &cli.RootOptions{},
				ContinueOnError: false,
				SkipMissing:     false,
			},
			updates: true,
			assocs: image.AssociationSet{"imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": map[string]image.Association{
				"imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19": {
					Name:            "imgname@sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Path:            "single_manifest",
					TagSymlink:      "latest",
					ID:              "sha256:d31c6ea5c50be93d6eb94d2b508f0208e84a308c011c6454ebf291d48b37df19",
					Type:            image.TypeGeneric,
					ManifestDigests: nil,
					LayerDigests: []string{
						"sha256:e8614d09b7bebabd9d8a450f44e88a8807c98a438a2ddd63146865286b132d1b",
						"sha256:601401253d0aac2bc95cccea668761a6e69216468809d1cee837b2e8b398e241",
						"sha256:211941188a4f55ffc6bcefa4f69b69b32c13fafb65738075de05808bbfcec086",
						"sha256:f0fd5be261dfd2e36d01069a387a3e5125f5fd5adfec90f3cb190d1d5f1d1ad9",
						"sha256:0c0beb258254c0566315c641b4107b080a96fa78d4f96833453dd6c5b9edf2b7",
						"sha256:30c794a11b4c340c77238c5b7ca845752904bd8b74b73a9b16d31253234da031",
					},
				},
			},
			},
			meta: v1alpha2.Metadata{
				MetadataSpec: v1alpha2.MetadataSpec{
					PastAssociations: []image.Association{},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			tmpdir := t.TempDir()
			path := filepath.Join(tmpdir, config.SourceDir, config.V2Dir)
			c.opts.RootOptions.Dir = tmpdir
			c.opts.OutputDir = tmpdir
			err := os.MkdirAll(path, os.ModePerm)
			require.NoError(t, err)
			require.NoError(t, copyV2(filepath.Join("testdata", config.V2Dir), path))
			ctx := context.Background()

			// First run will create mirror_seq1_0000.tar
			_, err = c.opts.Pack(ctx, c.assocs, &c.meta, 0)
			t.Log(err)

			if c.updates {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrNoUpdatesExist)
				// should not produce an archive
				_, err = os.Stat(filepath.Join(path, "mirror_seq1_000000.tar"))
				require.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}

func copyV2(source, destination string) error {
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		relPath := strings.Replace(path, source, "", 1)
		if relPath == "" {
			return nil
		}
		switch m := info.Mode(); {
		case m&fs.ModeSymlink != 0: // Tag is the file name, so follow the symlink to the layer ID-named file.
			dst, err := os.Readlink(path)
			if err != nil {
				return err
			}
			id := filepath.Base(dst)
			if err := os.Symlink(id, filepath.Join(destination, relPath)); err != nil {
				return err
			}
		case m.IsDir():
			return os.Mkdir(filepath.Join(destination, relPath), 0750)
		default:
			data, err := ioutil.ReadFile(filepath.Join(source, relPath))
			if err != nil {
				return err
			}
			return ioutil.WriteFile(filepath.Join(destination, relPath), data, 0777)
		}
		return nil
	})
	return err
}
