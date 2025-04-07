package archive

import (
	"context"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/assert"

	"github.com/openshift/oc-mirror/v2/internal/pkg/common"
	"github.com/openshift/oc-mirror/v2/internal/pkg/consts"
	"github.com/openshift/oc-mirror/v2/internal/pkg/errortype"
	"github.com/openshift/oc-mirror/v2/internal/pkg/mirror"
)

func TestImageBlobGatherer_GatherBlobs(t *testing.T) {
	type image struct {
		srcProtocol    string
		src            string
		destProtocol   string
		dest           string
		isParentImage  bool
		isManifestList bool
	}

	type testCase struct {
		caseName          string
		images            []image
		removeSignatures  bool
		expectedBlobs     map[string]struct{}
		expectedErrorType error
	}

	testCases := []testCase{
		{
			caseName: "image without signature and --remove-signature=true: pass",
			images: []image{
				{
					srcProtocol:   consts.DirProtocol,
					src:           "noo-bundle-image",
					destProtocol:  consts.DockerProtocol,
					dest:          "/test:latest",
					isParentImage: true,
				},
			},
			removeSignatures: true,
			expectedBlobs: map[string]struct{}{
				"sha256:467829ca4ff134ef9762a8f69647fdf2515b974dfc94a8474c493a45ef922e51": {},
				"sha256:728191dbaae078c825ffb518e15d33956353823d4da6c2e81fe9b1ed60ddef7d": {},
				"sha256:50b9402635dd4b312a86bed05dcdbda8c00120d3789ec2e9b527045100b3bdb4": {},
			},
		},
		{
			caseName: "image without signature and --remove-signature=false: image blobs should pass and signature blobs should fail",
			images: []image{
				{
					srcProtocol:   consts.DirProtocol,
					src:           "noo-bundle-image",
					destProtocol:  consts.DockerProtocol,
					dest:          "/test:latest",
					isParentImage: true,
				},
			},
			expectedErrorType: errortype.SignatureBlobGathererError{},
			expectedBlobs: map[string]struct{}{
				"sha256:467829ca4ff134ef9762a8f69647fdf2515b974dfc94a8474c493a45ef922e51": {},
				"sha256:728191dbaae078c825ffb518e15d33956353823d4da6c2e81fe9b1ed60ddef7d": {},
				"sha256:50b9402635dd4b312a86bed05dcdbda8c00120d3789ec2e9b527045100b3bdb4": {},
			},
		},
		{
			caseName: "single arch image with signatures: pass",
			images: []image{
				{
					srcProtocol:   consts.OciProtocol,
					src:           "single-arch-empty-image",
					destProtocol:  consts.DockerProtocol,
					dest:          "/empty-image:latest",
					isParentImage: true,
				},
				{
					srcProtocol:  consts.OciProtocol,
					src:          "single-arch-empty-image-sig",
					destProtocol: consts.DockerProtocol,
					dest:         "/empty-image:sha256-7d27ba1da0f64410102468d138f9ec8f61f2cc23bb58ff4cd63243a4434e3d99.sig",
				},
			},
			expectedBlobs: map[string]struct{}{
				"sha256:7d27ba1da0f64410102468d138f9ec8f61f2cc23bb58ff4cd63243a4434e3d99": {},
				"sha256:a890cd283a99877f5407c8f78fa4dae6af6e4a5def4a5383a343079bc89f309e": {},
				"sha256:f5160c724874c865c42578e560b8bd4c2172b0f8ec5b8192f4fc45b746d41962": {},
				"sha256:ed81f580f1cc03b20cb0600084a76b3da35a9e303ffc1b908e5b5e61e05d9f27": {},
				"sha256:1faf95412673f2c4345d6961ed543c92173589cfe364282f129d5de056003980": {},
				"sha256:5e7a034f575bb1b9188e4ea29470a2b86c640170125fa8c7ec9dc9c4bc17a373": {},
			},
		},
		{
			caseName: "multi arch image with signatures: pass",
			images: []image{
				{
					srcProtocol:    consts.OciProtocol,
					src:            "multi-platform-container-latest",
					destProtocol:   consts.DockerProtocol,
					dest:           "/multi-platform-container:latest",
					isParentImage:  true,
					isManifestList: true,
				},
				{
					srcProtocol:  consts.OciProtocol,
					src:          "multi-platform-container-sha256-0de0e983a4980f32b1aad2d7f7f387cea2d4e9517b47f336cef27f63735911fa-sig",
					destProtocol: consts.DockerProtocol,
					dest:         "/multi-platform-container:sha256-0de0e983a4980f32b1aad2d7f7f387cea2d4e9517b47f336cef27f63735911fa.sig",
				},
				{
					srcProtocol:  consts.OciProtocol,
					src:          "multi-platform-container-sha256-02f29c270f30416a266571383098d7b98a49488723087fd917128045bcd1ca75-sig",
					destProtocol: consts.DockerProtocol,
					dest:         "/multi-platform-container:sha256-02f29c270f30416a266571383098d7b98a49488723087fd917128045bcd1ca75.sig",
				},
				{
					srcProtocol:  consts.OciProtocol,
					src:          "multi-platform-container-sha256-832f20ad3d7e687c581b0a7d483174901d8bf22bb96c981b3f9da452817a754e-sig",
					destProtocol: consts.DockerProtocol,
					dest:         "/multi-platform-container:sha256-832f20ad3d7e687c581b0a7d483174901d8bf22bb96c981b3f9da452817a754e.sig",
				},
				{
					srcProtocol:  consts.OciProtocol,
					src:          "multi-platform-container-sha256-b15a2f174d803fd5fd7db0b3969c75cee0fe9131e0d8478f8c70ac01a4534869-sig",
					destProtocol: consts.DockerProtocol,
					dest:         "/multi-platform-container:sha256-b15a2f174d803fd5fd7db0b3969c75cee0fe9131e0d8478f8c70ac01a4534869.sig",
				},
				{
					srcProtocol:  consts.OciProtocol,
					src:          "multi-platform-container-sha256-e033aa62f84267cf44de611acac2e76bfa4d2f0b6b2b61f1c4fecbefefde7159-sig",
					destProtocol: consts.DockerProtocol,
					dest:         "/multi-platform-container:sha256-e033aa62f84267cf44de611acac2e76bfa4d2f0b6b2b61f1c4fecbefefde7159.sig",
				},
			},
			expectedBlobs: map[string]struct{}{
				"sha256:0de0e983a4980f32b1aad2d7f7f387cea2d4e9517b47f336cef27f63735911fa": {},
				"sha256:94328d9ae45c60c4c791f0066042521d57654bfd8e497b1cfbea3003ff3abb29": {},
				"sha256:84c87436d1101432e5af3ea09e89324afff39b06496849c1165930374167078f": {},
				"sha256:a5888bc451c329a02074f29a7f9d0710f117930907ffcf80f19f3836aabc8527": {},
				"sha256:832f20ad3d7e687c581b0a7d483174901d8bf22bb96c981b3f9da452817a754e": {},
				"sha256:f859b423345efd29f20d220aa79d8aaacf5fd6d1f1518ab42ed3ce35eb46688e": {},
				"sha256:cca6216db43b3ffa04aebbe9c3eb91de78e31301c7e015a7392fa79dfc85ce61": {},
				"sha256:fa50f89548925bace553298581f24c5c7a1297f3f95d5a15f1ee1f03b95afe10": {},
				"sha256:874b6ca0d55c403f9d1a782e1a143c4801f6cb1ca0cadb520e9df4a877eb914a": {},
				"sha256:f2a85fb3e895978775ec31bc0375e295e1ef08d8e6e323c5a74a947da1ad71c1": {},
				"sha256:b15a2f174d803fd5fd7db0b3969c75cee0fe9131e0d8478f8c70ac01a4534869": {},
				"sha256:645e7ac88707b388674231b8e473a29c38bcb60e764ea69e8b4f5791b9efd848": {},
				"sha256:fbbb412beb1de15db239cdf6211b41510b7148e2ae8bb6b7fa8595a749c7dfc4": {},
				"sha256:9af5f463e252eb32b3db3993ae017a21f2096fc867a1a9c41e6a37746fede539": {},
				"sha256:681624245c5af9557670a5ac63535149d930be055156bf617d9f5cd225a5eb4d": {},
				"sha256:0a43bd4effe1b9095d9110575b52bf237205585431d4eeac4db03c57c08c4562": {},
				"sha256:02f29c270f30416a266571383098d7b98a49488723087fd917128045bcd1ca75": {},
				"sha256:b1a8f85a8bf03345f76c684778bb4e56bf2e540d103bb87bf82c81e7139e2f3f": {},
				"sha256:e92ae2b67386a079de2f99e6045637f2612d9aa9b0089736152c060b338ba5b3": {},
				"sha256:fd23bc8220beb3fd8b52434f65c146345f1a1f409577c19326a30b3d4ccebba6": {},
				"sha256:aaf8e64a632cddf51e5b6b02784d635be5c8f39d36ee2361ef0b0de5f9824426": {},
				"sha256:89a25c4b082b1f78379293c7443585b598f34257dbb8bb2c7c33d93a30a45c52": {},
				"sha256:e033aa62f84267cf44de611acac2e76bfa4d2f0b6b2b61f1c4fecbefefde7159": {},
				"sha256:6db70183199d1d7a1c7ae19c982988c82180aa10383e7ebbbc9071d7a6cae87d": {},
				"sha256:e8170da5e917b5a8fd29726001e996f567f248541fde0f7bf42a4c7a5cca9ed0": {},
				"sha256:4ec5da06666e387046c4190d3ff21cff504736725b10845edcd8d103ffc5fa4e": {},
				"sha256:d1b3a652c935f8bdc538553d49cfe52aba8193adac103e63a78aebde96bbb1fa": {},
				"sha256:b690db3637d45cca671a5121ec22adc21226354d2b22ea03541e71efbe41b083": {},
			},
		},
	}

	testFolder := t.TempDir()
	ctx := context.Background()
	global := &mirror.GlobalOptions{WorkingDir: testFolder}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	srcFlags, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	destFlags, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	_ = srcFlags.Set("src-tls-verify", "false")
	_ = destFlags.Set("dest-tls-verify", "false")

	opts := mirror.CopyOptions{
		Global:    global,
		SrcImage:  srcOpts,
		DestImage: destOpts,
		RetryOpts: retryOpts,
		Mode:      mirror.MirrorToDisk,
	}
	// Set up a fake registry.
	s := httptest.NewServer(registry.New())
	defer s.Close()
	u, err := url.Parse(s.URL)
	assert.NoError(t, err)

	for _, tc := range testCases {
		var parentImage image
		for _, image := range tc.images {
			imgSrc, err := filepath.Abs(common.TestFolder + image.src)
			assert.NoError(t, err)

			src := image.srcProtocol + imgSrc
			dest := image.destProtocol + u.Host + image.dest

			opts.RemoveSignatures = tc.removeSignatures
			opts.All = image.isManifestList

			err = mirror.New(mirror.NewMirrorCopy(), mirror.NewMirrorDelete()).Run(ctx, src, dest, "copy", &opts)
			assert.NoError(t, err)

			if image.isParentImage {
				parentImage = image
			}
		}

		gatherer := NewImageBlobGatherer(&opts)

		blobs, err := gatherer.GatherBlobs(ctx, parentImage.destProtocol+u.Host+parentImage.dest)
		if tc.expectedErrorType != nil {
			assert.True(t, reflect.TypeOf(err) == reflect.TypeOf(tc.expectedErrorType))
		} else {
			assert.NoError(t, err, "GatherBlobs failed: %v", err)
		}

		assert.Equal(t, tc.expectedBlobs, blobs)
	}
}

func TestImageBlobGatherer_ImgRefError(t *testing.T) {
	ctx := context.Background()
	global := &mirror.GlobalOptions{
		SecurePolicy: false,
		Force:        true,
		WorkingDir:   "tests",
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	_, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	_, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	opts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
	}

	gatherer := NewImageBlobGatherer(&opts)
	_, err := gatherer.GatherBlobs(ctx, "error")
	assert.Equal(t, "invalid source name error: Invalid image name \"error\", expected colon-separated transport:reference", err.Error())

}

func TestImageBlobGatherer_SrcContextError(t *testing.T) {
	ctx := context.Background()
	global := &mirror.GlobalOptions{
		SecurePolicy: false,
		Force:        true,
		WorkingDir:   "tests",
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	srcFlags, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, nil, "src-", "screds")
	destFlags, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	_ = srcFlags.Set("src-tls-verify", "false")
	_ = destFlags.Set("dest-tls-verify", "false")

	opts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
	}

	gatherer := NewImageBlobGatherer(&opts)
	_, err := gatherer.GatherBlobs(ctx, "docker://localhost/test:latest")
	assert.Equal(t, "error when creating a new image source pinging container registry localhost: Get \"http://localhost/v2/\": dial tcp [::1]:80: connect: connection refused", err.Error())

}

func TestImageBlobGatherer_ImageSourceError(t *testing.T) {
	ctx := context.Background()
	global := &mirror.GlobalOptions{
		SecurePolicy: false,
		Force:        true,
		WorkingDir:   "tests",
	}

	_, sharedOpts := mirror.SharedImageFlags()
	_, deprecatedTLSVerifyOpt := mirror.DeprecatedTLSVerifyFlags()
	srcFlags, srcOpts := mirror.ImageSrcFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "src-", "screds")
	destFlags, destOpts := mirror.ImageDestFlags(global, sharedOpts, deprecatedTLSVerifyOpt, "dest-", "dcreds")
	_, retryOpts := mirror.RetryFlags()

	_ = srcFlags.Set("src-tls-verify", "false")
	_ = destFlags.Set("dest-tls-verify", "false")

	opts := mirror.CopyOptions{
		Global:              global,
		DeprecatedTLSVerify: deprecatedTLSVerifyOpt,
		SrcImage:            srcOpts,
		DestImage:           destOpts,
		RetryOpts:           retryOpts,
		Dev:                 false,
		Mode:                mirror.MirrorToDisk,
	}
	// Set up a fake registry.
	s := httptest.NewServer(registry.New())
	defer s.Close()
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}

	gatherer := NewImageBlobGatherer(&opts)
	_, err = gatherer.GatherBlobs(ctx, "docker://"+u.Host+"/bad-test:latest")
	assert.Contains(t, err.Error(), "name unknown: Unknown name")

}
