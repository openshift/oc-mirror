package mirror

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	imagecopy "github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/types"
	"github.com/google/go-containerregistry/pkg/crane"
	gocontreg "github.com/google/go-containerregistry/pkg/v1"
	gocontregtypes "github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/openshift/library-go/pkg/image/reference"
	"github.com/openshift/oc-mirror/pkg/api/v1alpha2"
	"github.com/openshift/oc-mirror/pkg/cli"
	"github.com/openshift/oc-mirror/pkg/image"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/otiai10/copy"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	testdata        = "testdata/artifacts/rhop-ctlg-oci"
	testdata_mashed = "testdata/artifacts/rhop-ctlg-oci-mashed"
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
			_, err := c.options.findFBCConfig(c.options.OutputDir, filepath.Join(c.options.OutputDir, artifactsFolderName))
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
		expectedRelatedImages []declcfg.RelatedImage
		packages              []v1alpha2.IncludePackage
		err                   string
	}
	tmpdir := t.TempDir()
	cases := []spec{
		{
			desc:        "nominal case",
			configsPath: filepath.Join(testdata, blobsPath, "cac5b2f40be10e552461651655ca8f3f6ba3f65f41ecf4345efbcf1875415db6"),
			packages: []v1alpha2.IncludePackage{
				{
					Name: "node-observability-operator",
				},
			},
			expectedRelatedImages: []declcfg.RelatedImage{
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
				{
					Name:  "kube-rbac-proxy",
					Image: "registry.redhat.io/openshift4/ose-kube-rbac-proxy@sha256:bb54bc66185afa09853744545d52ea22f88b67756233a47b9f808fe59cda925e",
				},
			},
			err: "",
		},
		{
			desc:        "nominal mashed case",
			configsPath: filepath.Join(testdata_mashed, blobsPath, "cac5b2f40be10e552461651655ca8f3f6ba3f65f41ecf4345efbcf1875415db6"),
			packages: []v1alpha2.IncludePackage{
				{
					Name: "foo",
				},
			},
			expectedRelatedImages: []declcfg.RelatedImage{
				{
					Image: "quay.io/redhatgov/oc-mirror-dev@sha256:7d6bfc2e0450e0fe7a469884e5078b95c44ef84843e66bbf4ae0b3db17eeae84",
					Name:  "operator",
				},
				{
					Image: "quay.io/redhatgov/oc-mirror-dev@sha256:4daee43355c22f037602e266f012c158fd567050ad6c9dc36e49a45012196d62",
					Name:  "operator",
				},
				{
					Name:  "operator",
					Image: "quay.io/redhatgov/oc-mirror-dev@sha256:7e1e74b87a503e95db5203334917856f61aece90a72e8d53a9fd903344eb78a5",
				},
				{Name: "operator",
					Image: "quay.io/redhatgov/oc-mirror-dev@sha256:00aef3f7bd9bea8f627dbf46d2d062010ed7d8b208a98da389b701c3cae90026",
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
			relatedImages, err := getRelatedImages(filepath.Join(tmpdir, "configs"), c.packages)
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

func TestPullImage(t *testing.T) {
	type spec struct {
		desc        string
		from        string
		to          string
		stl         style
		funcs       RemoteRegFuncs
		expectedErr string
	}
	cases := []spec{
		{
			desc:        "nominal oci case passes",
			to:          ociProtocol + t.TempDir(),
			from:        "docker://localhost:5000/ocmir/a-fake-image:latest",
			stl:         ociStyle,
			funcs:       createMockFunctions(),
			expectedErr: "",
		},
		{
			desc:        "nominal non-oci case passes",
			to:          ociProtocol + t.TempDir(),
			from:        "docker://localhost:5000/ocmir/a-fake-image:latest",
			stl:         originStyle,
			funcs:       createMockFunctions(),
			expectedErr: "",
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			err := pullImage(c.from, c.to, false, c.stl, c.funcs)
			if c.expectedErr != "" {
				require.EqualError(t, err, c.expectedErr)
			} else {
				require.NoError(t, err)
			}

		})
	}
}

func TestPushImage(t *testing.T) {
	type spec struct {
		desc        string
		from        string
		to          string
		funcs       RemoteRegFuncs
		expectedErr string
	}
	cases := []spec{
		{
			desc:        "nominal case passes",
			from:        ociProtocol + testdata,
			to:          "docker://localhost:5000/ocmir",
			funcs:       createMockFunctions(),
			expectedErr: "",
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			err := pushImage(c.from, c.to, true, true, c.funcs)
			if c.expectedErr != "" {
				require.EqualError(t, err, c.expectedErr)
			} else {
				require.NoError(t, err)
			}

		})
	}
}

func TestGetISConfig(t *testing.T) {
	type spec struct {
		desc        string
		isc         *v1alpha2.ImageSetConfiguration
		options     *MirrorOptions
		err         string
		expectedErr string
	}
	c := spec{
		desc: "nominal case passes",
		options: &MirrorOptions{
			UseOCIFeature:    true,
			OCIFeatureAction: OCIFeatureCopyAction,
			RootOptions: &cli.RootOptions{
				Dir: "",
				IOStreams: genericclioptions.IOStreams{
					In:     os.Stdin,
					Out:    os.Stdout,
					ErrOut: os.Stderr,
				},
			},
			ConfigPath: "testdata/configs/iscfg.yaml",
		},
		expectedErr: "",
	}
	t.Run(c.desc, func(t *testing.T) {
		_, err := c.options.getISConfig()

		if c.expectedErr != "" {
			require.EqualError(t, err, c.err)
		} else {
			require.NoError(t, err)
		}
	})
}

func TestBulkImageCopy(t *testing.T) {
	type spec struct {
		desc               string
		isc                *v1alpha2.ImageSetConfiguration
		expectedSubFolders []string
		options            *MirrorOptions
		funcs              RemoteRegFuncs

		err string
	}

	cases := []spec{
		{
			desc: "Nominal case passes",
			isc: &v1alpha2.ImageSetConfiguration{
				TypeMeta: v1alpha2.NewMetadata().TypeMeta,
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{

						Operators: []v1alpha2.Operator{
							{
								Catalog: "docker://registry.redhat.io/openshift/fakecatalog:latest",
								IncludeConfig: v1alpha2.IncludeConfig{
									Packages: []v1alpha2.IncludePackage{
										{
											Name: "aws-load-balancer-operator",
											Channels: []v1alpha2.IncludeChannel{
												{
													Name: "stable-v0.1",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			options: &MirrorOptions{
				From:             "test.registry.io",
				ToMirror:         "",
				UseOCIFeature:    true,
				OCIFeatureAction: OCIFeatureCopyAction,
				OutputDir:        "",
				RootOptions: &cli.RootOptions{
					Dir: "",
					IOStreams: genericclioptions.IOStreams{
						In:     os.Stdin,
						Out:    os.Stdout,
						ErrOut: os.Stderr,
					},
				},
				SourceSkipTLS: true,
				DestSkipTLS:   true,
			},
			funcs:              createMockFunctions(),
			err:                "",
			expectedSubFolders: []string{"aws-load-balancer-operator"},
		},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			tmpDir := t.TempDir()
			c.options.OutputDir = tmpDir
			c.options.Dir = filepath.Join(tmpDir, "oc-mirror-workspace")
			err := c.options.bulkImageCopy(c.isc, c.options.SourceSkipTLS, c.options.DestSkipTLS, c.funcs)
			if c.err != "" {
				require.EqualError(t, err, c.err)
			} else {
				require.NoError(t, err)

			}
		})
	}
}

func TestBulkImageMirror(t *testing.T) {
	type spec struct {
		desc               string
		isc                *v1alpha2.ImageSetConfiguration
		expectedSubFolders []string
		options            *MirrorOptions
		funcs              RemoteRegFuncs

		err string
	}

	cases := []spec{
		{
			desc: "Nominal case passes",
			isc: &v1alpha2.ImageSetConfiguration{
				TypeMeta: v1alpha2.NewMetadata().TypeMeta,
				ImageSetConfigurationSpec: v1alpha2.ImageSetConfigurationSpec{
					Mirror: v1alpha2.Mirror{

						Operators: []v1alpha2.Operator{
							{
								Catalog: "oci://" + testdata,
								IncludeConfig: v1alpha2.IncludeConfig{
									Packages: []v1alpha2.IncludePackage{
										{
											Name: "aws-load-balancer-operator",
											Channels: []v1alpha2.IncludeChannel{
												{
													Name: "stable-v0.1",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			options: &MirrorOptions{
				From:             testdata,
				ToMirror:         "localhost.localdomain:5000",
				UseOCIFeature:    true,
				OCIFeatureAction: OCIFeatureMirrorAction,
				OutputDir:        "",
				RootOptions: &cli.RootOptions{
					Dir: "",
					IOStreams: genericclioptions.IOStreams{
						In:     os.Stdin,
						Out:    os.Stdout,
						ErrOut: os.Stderr,
					},
				},
				SourceSkipTLS:              true,
				DestSkipTLS:                true,
				OCIInsecureSignaturePolicy: true,
			},
			funcs:              createMockFunctions(),
			err:                "",
			expectedSubFolders: []string{"aws-load-balancer-operator"},
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			tmpDir := t.TempDir()
			c.options.OutputDir = tmpDir
			c.options.Dir = filepath.Join(tmpDir, "oc-mirror-workspace")
			err := c.options.bulkImageMirror(c.isc, c.options.ToMirror, "testnamespace", c.funcs)
			if c.err != "" {
				require.EqualError(t, err, c.err)
			} else {
				require.NoError(t, err)

			}
		})
	}
}

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

// ////////////////////   Fakes &  mocks ///////////////////////
type fakeCraneImg struct{}

func (f fakeCraneImg) Layers() ([]gocontreg.Layer, error) {
	return nil, nil
}
func (f fakeCraneImg) MediaType() (gocontregtypes.MediaType, error) {
	return "", nil
}
func (f fakeCraneImg) Size() (int64, error) {
	return 0, nil
}
func (f fakeCraneImg) ConfigName() (gocontreg.Hash, error) {
	return gocontreg.Hash{}, nil
}
func (f fakeCraneImg) ConfigFile() (*gocontreg.ConfigFile, error) {
	return nil, nil
}
func (f fakeCraneImg) RawConfigFile() ([]byte, error) {
	return nil, nil
}
func (f fakeCraneImg) Digest() (gocontreg.Hash, error) {
	return gocontreg.Hash{}, nil
}
func (f fakeCraneImg) Manifest() (*gocontreg.Manifest, error) {
	return nil, nil
}
func (f fakeCraneImg) RawManifest() ([]byte, error) {
	return nil, nil
}
func (f fakeCraneImg) LayerByDigest(gocontreg.Hash) (gocontreg.Layer, error) {
	return nil, nil
}
func (f fakeCraneImg) LayerByDiffID(gocontreg.Hash) (gocontreg.Layer, error) {
	return nil, nil
}

func createMockFunctions() RemoteRegFuncs {
	return RemoteRegFuncs{
		push: func(ctx context.Context, policyContext *signature.PolicyContext, destRef types.ImageReference, srcRef types.ImageReference, options *imagecopy.Options) (copiedManifest []byte, retErr error) {
			return nil, nil
		},
		load: func(path string, opt ...crane.Option) (gocontreg.Image, error) {
			return nil, nil
		},
		pull: func(src string, opt ...crane.Option) (gocontreg.Image, error) {
			img := fakeCraneImg{}
			return img, nil
		},
		saveOCI: func(img gocontreg.Image, path string) error {
			// copy testData to the path selected
			err := copy.Copy(trimProtocol(testdata), trimProtocol(path))
			return err
		},
		saveLegacy: func(img gocontreg.Image, src, path string) error {
			return nil
		},
		mirrorMappings: func(cfg v1alpha2.ImageSetConfiguration, images image.TypedImageMapping, insecure bool) error {
			return nil
		},
	}
}
