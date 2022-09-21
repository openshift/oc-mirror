package mirror

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	testdata        = "testdata/artifacts/rhop-ctlg-oci"
	rotten_manifest = "testdata/artifacts/rhop-rotten-manifest"
	rotten_layer    = "testdata/artifacts/rhop-rotten-layer"
	rotten_config   = "testdata/artifacts/rhop-rotten-cfg"
)

// TODO: add preparation step that saves a catalog locally before testing
// see maybe contents of pkg/image/testdata
func TestGetOCIImgSrcFromPath(t *testing.T) {
	type spec struct {
		desc  string
		inRef string
		err   string
	}
	wdir, err := os.Getwd()
	if err != nil {
		t.Fatal("unable to get working dir")
	}
	cases := []spec{
		{
			desc:  "full path passes",
			inRef: filepath.Join(wdir, testdata),
			err:   "",
		},
		{
			desc:  "relative path passes",
			inRef: testdata,
			err:   "",
		},
		{
			desc:  "inexisting path should fail",
			inRef: "/inexisting",
			err:   "unable to get OCI Image from /inexisting: open /inexisting/index.json: no such file or directory",
		},
		{
			desc:  "path not containing oci structure should fail",
			inRef: "/tmp",
			err:   "unable to get OCI Image from /tmp: open /tmp/index.json: no such file or directory",
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			imgSrc, err := getOCIImgSrcFromPath(context.TODO(), c.inRef)
			if c.err != "" {
				require.EqualError(t, err, c.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, "oci", imgSrc.Reference().Transport().Name())
				imgSrc.Close()
			}

		})
	}
}

func TestGetManifest(t *testing.T) {
	type spec struct {
		desc       string
		inRef      string
		layerCount int
		err        string
	}
	wdir, err := os.Getwd()
	if err != nil {
		t.Fatal("unable to get working dir")
	}
	cases := []spec{
		{
			desc:       "nominal case",
			inRef:      filepath.Join(wdir, testdata),
			layerCount: 1,
			err:        "",
		},
		{
			desc:       "index is unmarshallable fails",
			inRef:      filepath.Join(wdir, rotten_manifest),
			layerCount: 0,
			err:        "unable to unmarshall manifest of image : unexpected end of JSON input",
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			imgSrc, err := getOCIImgSrcFromPath(context.TODO(), c.inRef)
			if err != nil {
				t.Fatalf("The given path is not an OCI image : %v", err)
			}
			defer imgSrc.Close()
			manifest, err := getManifest(context.TODO(), imgSrc)
			if c.err != "" {
				require.EqualError(t, err, c.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, c.layerCount, len(manifest.LayerInfos()))
			}

		})
	}
}
func TestFindFBCConfig(t *testing.T) {
	type spec struct {
		desc    string
		options *MirrorOptions
		err     string
	}
	cases := []spec{
		{
			desc: "nominal case",
			options: &MirrorOptions{
				From:             ociProtocol + testdata,
				ToMirror:         "test.registry.io",
				UseOCIFeature:    true,
				OCIFeatureAction: OCIFeatureCopyAction,
				OutputDir:        testdata,
			},
			err: "",
		},
		{
			desc: "not a FBC image fails",
			options: &MirrorOptions{
				From:             ociProtocol + testdata,
				ToMirror:         "test.registry.io",
				UseOCIFeature:    true,
				OCIFeatureAction: OCIFeatureCopyAction,
				OutputDir:        "/tmp",
			},
			err: "unable to get OCI Image from /tmp: open /tmp/index.json: no such file or directory",
		},
		{
			desc: "corrupted manifest fails",
			options: &MirrorOptions{
				From:             ociProtocol + testdata,
				ToMirror:         "test.registry.io",
				UseOCIFeature:    true,
				OCIFeatureAction: OCIFeatureCopyAction,
				OutputDir:        rotten_manifest,
			},
			err: "unable to unmarshall manifest of image : unexpected end of JSON input",
		},
		{
			desc: "corrupted layer fails",
			options: &MirrorOptions{
				From:             ociProtocol + testdata,
				ToMirror:         "test.registry.io",
				UseOCIFeature:    true,
				OCIFeatureAction: OCIFeatureCopyAction,
				OutputDir:        rotten_layer,
			},
			err: "UntarLayers: NewReader failed - gzip: invalid header",
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			err := c.options.FindFBCConfig(c.options.OutputDir)
			if c.err != "" {
				require.EqualError(t, err, c.err)
			} else {
				require.NoError(t, err)
			}

		})
	}
}

func TestGetConfigPathFromLabel(t *testing.T) {
	type spec struct {
		desc            string
		imagePath       string
		configSha       string
		expectedDirName string
		err             string
	}
	cases := []spec{
		{
			desc:            "nominal case",
			imagePath:       testdata,
			configSha:       "sha256:c7c89df4a1f53d7e619080245c4784b6f5e6232fb71e98d981b89799ae578262",
			expectedDirName: "/configs",
			err:             "",
		},
		{
			desc:            "sha doesnt exist fails",
			imagePath:       testdata,
			configSha:       "sha256:inexistingSha",
			expectedDirName: "",
			err:             "unable to read the config blob inexistingSha from the oci image: open testdata/artifacts/rhop-ctlg-oci/blobs/sha256/inexistingSha: no such file or directory",
		},
		{
			desc:            "cfg layer json incorrect fails",
			imagePath:       rotten_config,
			configSha:       "sha256:c7c89df4a1f53d7e619080245c4784b6f5e6232fb71e98d981b89799ae578262",
			expectedDirName: "",
			err:             "problem unmarshaling config blob in c7c89df4a1f53d7e619080245c4784b6f5e6232fb71e98d981b89799ae578262: unexpected end of JSON input",
		},
		{
			desc:            "label doesnt exist fails",
			imagePath:       rotten_config,
			configSha:       "sha256:c7c89df4a1f53d7e619080245c4784b6f5e6232fb71e98d981b89799ae5782ff",
			expectedDirName: "",
			err:             "label " + configsLabel + " not found in config blob c7c89df4a1f53d7e619080245c4784b6f5e6232fb71e98d981b89799ae5782ff",
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			cfgDir, err := getConfigPathFromLabel(c.imagePath, c.configSha)
			if c.err != "" {
				require.EqualError(t, err, c.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, c.expectedDirName, cfgDir)
			}

		})
	}
}

func TestGetRelatedImages(t *testing.T) {}

func TestCopyImage(t *testing.T)       {}
func TestBulkImageMirror(t *testing.T) {}
func TestBulkImageCopy(t *testing.T)   {}
func TestUntarLayers(t *testing.T)     {}
