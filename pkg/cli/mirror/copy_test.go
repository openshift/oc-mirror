package mirror

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/operator-framework/operator-registry/alpha/model"
	"github.com/stretchr/testify/require"
)

const (
	testdata        = "testdata/artifacts/rhop-ctlg-oci"
	rotten_manifest = "testdata/artifacts/rhop-rotten-manifest"
	rotten_layer    = "testdata/artifacts/rhop-rotten-layer"
	rotten_config   = "testdata/artifacts/rhop-rotten-cfg"
	other_layer     = "testdata/artifacts/rhop-not-catalog"
)

func TestParse(t *testing.T) {
	toTest := "quay.io/skhoury/ocmir/albo/aws-load-balancer-controller-rhel8@sha256:d7bc364512178c36671d8a4b5a76cf7cb10f8e56997106187b0fe1f032670ece"
	s, err := reference.Parse(toTest)
	if err != nil {
		t.Fatalf("%v", err)
	}
	rf, err := image.ParseReference(toTest)

	if err != nil {
		t.Fatalf("%v", err)
	}
	fmt.Printf("%s - %s\n", s, rf)
}

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
			err:   "unable to get OCI Image from oci:/inexisting: open /inexisting/index.json: no such file or directory",
		},
		{
			desc:  "path not containing oci structure should fail",
			inRef: "/tmp",
			err:   "unable to get OCI Image from oci:/tmp: open /tmp/index.json: no such file or directory",
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
			cfgDir, err := getConfigPathFromConfigLayer(c.imagePath, c.configSha)
			if c.err != "" {
				require.EqualError(t, err, c.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, c.expectedDirName, cfgDir)
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
			err: "unable to get OCI Image from oci:/tmp: open /tmp/index.json: no such file or directory",
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
			_, err := c.options.findFBCConfig(c.options.OutputDir, "tmp")
			if c.err != "" {
				require.EqualError(t, err, c.err)
			} else {
				require.NoError(t, err)
			}

		})
	}
}

func TestGetRelatedImages(t *testing.T) {
	type spec struct {
		desc                  string
		configsPath           string
		expectedRelatedImages []model.RelatedImage
		err                   string
	}
	tmpdir := t.TempDir()
	cases := []spec{
		{
			desc:        "nominal case",
			configsPath: filepath.Join(testdata, blobsPath, "cac5b2f40be10e552461651655ca8f3f6ba3f65f41ecf4345efbcf1875415db6"),
			expectedRelatedImages: []model.RelatedImage{
				{
					Image: "registry.redhat.io/noo/node-observability-agent-rhel8@sha256:59bd5b8cefae5d5769d33dafcaff083b583a552e1df61194a3cc078b75cb1fdc",
					Name:  "agent",
				},
				{
					Image: "registry.redhat.io/noo/node-observability-operator-bundle-rhel8@sha256:25b8e1c8ed635364d4dcba7814ad504570b1c6053d287ab7e26c8d6a97ae3f6a",
					Name:  "",
				},
				{
					Name:  "node-observability-rhel8-operator-0040925e971e4bb3ac34278c3fb5c1325367fe41ad73641e6502ec2104bc4e19-annotation",
					Image: "registry.redhat.io/noo/node-observability-rhel8-operator@sha256:0040925e971e4bb3ac34278c3fb5c1325367fe41ad73641e6502ec2104bc4e19",
				},
				// {
				// 	Name:  "manager",
				// 	Image: "registry.redhat.io/noo/node-observability-rhel8-operator@sha256:0040925e971e4bb3ac34278c3fb5c1325367fe41ad73641e6502ec2104bc4e19",
				// },
				{
					Name:  "kube-rbac-proxy",
					Image: "registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:bb54bc66185afa09853744545d52ea22f88b67756233a47b9f808fe59cda925e",
				},
			},
			err: "",
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			//Untar the configs blob to tmpdir
			stream, err := os.Open(c.configsPath)
			if err != nil {
				t.Fatalf("unable to open %s: %v", c.configsPath, err)
			}
			err = UntarLayers(stream, tmpdir, "configs/")
			if err != nil {
				t.Fatalf("unable to untar %s: %v", c.configsPath, err)
			}
			relatedImages, err := getRelatedImages(filepath.Join(tmpdir, "configs", "node-observability-operator"))
			if c.err != "" {
				require.EqualError(t, err, c.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, len(c.expectedRelatedImages), len(relatedImages))
				require.ElementsMatch(t, c.expectedRelatedImages, relatedImages)
			}

		})
	}
}

func TestCopyImage(t *testing.T)       {}
func TestBulkImageMirror(t *testing.T) {}
func TestBulkImageCopy(t *testing.T)   {}
func TestUntarLayers(t *testing.T) {
	type spec struct {
		desc               string
		configsPath        string
		expectedSubFolders []string
		err                string
	}
	cases := []spec{
		{
			desc:               "nominal case",
			configsPath:        filepath.Join(testdata, blobsPath, "cac5b2f40be10e552461651655ca8f3f6ba3f65f41ecf4345efbcf1875415db6"),
			expectedSubFolders: []string{"node-observability-operator", "aws-load-balancer-operator"},
			err:                "",
		},
		{
			desc:               "layer is not a tar.gz fails",
			configsPath:        filepath.Join(rotten_layer, blobsPath, "1a6ae3d35ced1c7654b3bf1a66b8a513d2ee7f497728e0c5c74841807c4b8e77"),
			expectedSubFolders: nil,
			err:                "UntarLayers: NewReader failed - gzip: invalid header",
		},
		{
			desc:               "layer doesnt contain configs folder",
			configsPath:        filepath.Join(other_layer, blobsPath, "cac5b2f40be10e552461651655ca8f3f6ba3f65f41ecf4345efbcf1875415db6"),
			expectedSubFolders: []string{},
			err:                "",
		},
	}
	for _, c := range cases {
		tmpdir := t.TempDir()
		t.Run(c.desc, func(t *testing.T) {
			//Untar the configs blob to tmpdir
			stream, err := os.Open(c.configsPath)
			if err != nil {
				t.Fatalf("unable to open %s: %v", c.configsPath, err)
			}
			err = UntarLayers(stream, tmpdir, "configs/")
			if c.err != "" {
				require.EqualError(t, err, c.err)
			} else {
				require.NoError(t, err)
				f, err := os.Open(filepath.Join(tmpdir, "configs"))
				if err != nil && len(c.expectedSubFolders) == 0 {
					//here the filter caught 0 configs folder, so the error is normal
					return
				} else if err != nil && len(c.expectedSubFolders) > 0 {
					t.Errorf("unable to open the untarred folder: %v", err)
					t.Fail()
				}
				subfolders, err := f.Readdir(0)
				if err != nil {
					t.Errorf("unable to read untarred folder contents: %v", err)
					t.Fail()
				}
				require.Equal(t, len(c.expectedSubFolders), len(subfolders))
				for _, sf := range subfolders {
					require.Contains(t, c.expectedSubFolders, sf.Name())
				}
			}
		})
	}
}
