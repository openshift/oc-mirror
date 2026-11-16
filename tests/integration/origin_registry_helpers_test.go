package integration_test

import (
	_ "embed"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/oc-mirror/tests/integration/pkg/fixtures"
	"github.com/openshift/oc-mirror/tests/integration/pkg/origin"
)

// goMod pins the opm base image of the catalogs to the operator-registry version used by the tests.
//
//go:embed go.mod
var goMod []byte

const (
	// fixtureReleaseRepo and fixtureReleaseTag locate the releases built from testdata/releases and
	// their components, built from testdata/release-components.
	fixtureReleaseRepo = "oc-mirror/release"
	fixtureReleaseTag  = "v0.0.1"
)

var (
	linuxAMD64   = v1.Platform{OS: "linux", Architecture: "amd64"}
	linuxARM64   = v1.Platform{OS: "linux", Architecture: "arm64"}
	linuxS390X   = v1.Platform{OS: "linux", Architecture: "s390x"}
	linuxPPC64LE = v1.Platform{OS: "linux", Architecture: "ppc64le"}

	// multiArchFixtures lists the images of testdata that are built as an index for several
	// platforms, by directory name. The other ones are built as a linux/amd64 image.
	multiArchFixtures = map[string][]v1.Platform{
		"test-catalog-multi":       {linuxAMD64, linuxARM64},
		"foo-bundle-v0.3.1-multi":  {linuxAMD64, linuxARM64},
		"bar-bundle-v1.0.0-multi":  {linuxAMD64, linuxARM64},
		"test-release-index-multi": {linuxAMD64, linuxARM64, linuxS390X, linuxPPC64LE},
		"test-image-multi":         {linuxAMD64, linuxARM64},
	}
)

// The catalog and release images, and the images they reference, are built from testdata when
// the suite starts, and served by the origin registry, so that changing them does not require
// publishing anything.
var (
	originRegistry *origin.Registry

	// happyPathCatalog is the test-catalog image, from testdata/catalogs/test-catalog.
	happyPathCatalog string
	// testReleaseImage and multiReleaseImage are the release images of testdata/releases.
	testReleaseImage  string
	multiReleaseImage string
	// releaseComponentImage is the component of testReleaseImage, from testdata/release-components.
	releaseComponentImage string
	// operandImage is the operand every bundle refers to by digest, from testdata/operands/operand.
	operandImage string
)

// startOriginRegistry builds every image of testdata, and serves them, signed, over HTTPS like a
// public registry. oc-mirror and the tests trust its certificate, and oc-mirror trusts the GPG key
// of the release signatures.
func startOriginRegistry(home string) {
	opmImage, err := fixtures.OPMImage(goMod)
	Expect(err).NotTo(HaveOccurred())
	originRegistry, err = origin.Start(ctx, origin.Config{
		RegistryConfig: registryConfig,
		OperatorRepo:   testCatalogRepo,
		CatalogsDir:    filepath.Join(testdataDir, "catalogs"),
		BundlesDir:     filepath.Join(testdataDir, "bundles"),
		OperandsDir:    filepath.Join(testdataDir, "operands"),
		ReleaseRepo:    fixtureReleaseRepo,
		ReleaseTag:     fixtureReleaseTag,
		ReleasesDir:    filepath.Join(testdataDir, "releases"),
		ComponentsDir:  filepath.Join(testdataDir, "release-components"),
		MultiArch:      multiArchFixtures,
		OPMImage:       opmImage,
		CosignKey:      filepath.Join(keysDir, "cosign.key"),
		WorkDir:        GinkgoT().TempDir(),
	})
	Expect(err).NotTo(HaveOccurred(), "failed to build the images of testdata")
	DeferCleanup(originRegistry.Stop)

	// oc-mirror reads the certificate from the containers configuration in home, and the tests
	// read source images with the default go-containerregistry transport. That transport is
	// modified rather than replaced, because helpers like sourceDigestTransport share it.
	Expect(originRegistry.InstallCertificate(home)).To(Succeed())
	remote.DefaultTransport.(*http.Transport).TLSClientConfig = originRegistry.ClientTLSConfig()
	GinkgoT().Setenv("OCP_SIGNATURE_VERIFICATION_PK", originRegistry.ReleasePublicKeyFile())

	happyPathCatalog = fixtureImage(testCatalogRepo + ":" + testCatalogTag)
	testReleaseImage = fixtureImage(fixtureReleaseRepo + "/test-release-index:" + fixtureReleaseTag)
	multiReleaseImage = fixtureImage(fixtureReleaseRepo + "/test-release-index-multi:" + fixtureReleaseTag)
	releaseComponentImage = fixtureImage(fixtureReleaseRepo + "/test-image:" + fixtureReleaseTag)
	operandImage = fixtureByDigest(fixtureImage(testCatalogRepo + ":operand"))
}

// fixtureImage returns the reference of an image built from testdata, given its repository and tag.
func fixtureImage(repoTag string) string {
	GinkgoHelper()
	ref, err := originRegistry.Image(repoTag)
	Expect(err).NotTo(HaveOccurred())
	return ref
}

// fixtureByDigest returns the reference by digest of an image built from testdata, given by tag.
func fixtureByDigest(imageRef string) string {
	GinkgoHelper()
	ref, err := originRegistry.ByDigest(imageRef)
	Expect(err).NotTo(HaveOccurred())
	return ref.Name()
}

// bundleImage returns the reference of the bundle built from testdata/bundles/<bundle>.
func bundleImage(bundle string) string {
	GinkgoHelper()
	return fixtureImage(testCatalogRepo + ":" + bundle)
}

// digestOf returns the digest of an image reference by digest.
func digestOf(imageRef string) string {
	GinkgoHelper()
	ref, err := name.NewDigest(imageRef)
	Expect(err).NotTo(HaveOccurred())
	return ref.DigestStr()
}

// digestTag returns the tag oc-mirror gives to an image it mirrors by digest, like sha256-<hex>.
func digestTag(imageRef string) string {
	GinkgoHelper()
	return strings.Replace(digestOf(imageRef), ":", "-", 1)
}

// renderImageSetConfigs copies srcDir to a temporary directory, replacing the placeholders that
// refer to the images built by the suite, and returns the copy.
func renderImageSetConfigs(srcDir string) string {
	dstDir := GinkgoT().TempDir()
	Expect(originRegistry.RenderDir(srcDir, dstDir)).To(Succeed(), "failed to render the ImageSetConfigurations")
	return dstDir
}

// withOriginRegistryOffline stops the origin registry while run executes. Offline tests use it with
// unreachable proxies, so that oc-mirror cannot pull any image from its source.
func withOriginRegistryOffline(run func()) {
	GinkgoHelper()
	// The idle connections of the tests would only delay the shutdown.
	remote.DefaultTransport.(*http.Transport).CloseIdleConnections()
	originRegistry.Transport().(*http.Transport).CloseIdleConnections()
	err := originRegistry.Suspend()
	// Restart it even if the shutdown failed, which closes it anyway, for the next tests.
	defer func() {
		Expect(originRegistry.Resume(ctx)).To(Succeed(), "failed to restart the origin registry")
	}()
	Expect(err).NotTo(HaveOccurred(), "failed to stop the origin registry")
	run()
}
