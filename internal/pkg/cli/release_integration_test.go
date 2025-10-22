package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otiai10/copy"

	clog "github.com/openshift/oc-mirror/v2/internal/pkg/log"
	"github.com/openshift/oc-mirror/v2/internal/testutils"
	"github.com/stretchr/testify/assert"
)

type TestEnvironmentRelease struct {
	sourceServer              *httptest.Server
	destinationServer         *httptest.Server
	sourceRegistryDomain      string
	destinationRegistryDomain string
	tempFolder                string
	imageSetConfig            string
	cincinnatiServer          *httptest.Server
	cincinnatiEndpoint        string
	releaseImageRefs          []string
}

// before all the tests
func setupReleaseTest(t *testing.T) TestEnvironmentRelease {
	suite := TestEnvironmentRelease{}
	// setup source registry
	suite.sourceServer = testutils.CreateRegistry()

	us, err := url.Parse(suite.sourceServer.URL)
	assert.NoError(t, err, "should not fail to get url of source registry")

	suite.sourceRegistryDomain = us.Host

	// setup destination registry
	suite.destinationServer = testutils.CreateRegistry()

	ud, err := url.Parse(suite.destinationServer.URL)
	assert.NoError(t, err, "should not fail to get url of source registry")

	suite.destinationRegistryDomain = ud.Host

	suite.tempFolder = t.TempDir()
	suite.setupTestData(t)

	return suite
}

func (suite *TestEnvironmentRelease) tearDown() {

	os.RemoveAll(suite.tempFolder)
	suite.sourceServer.Close()
	suite.destinationServer.Close()
}

func (suite *TestEnvironmentRelease) copyArchiveForD2M(t *testing.T) {
	d2mPath := filepath.Join(suite.tempFolder, "release", d2mSubFolder)
	err := os.MkdirAll(d2mPath, 0755)
	assert.NoError(t, err, "should not fail creating "+d2mPath)
	archivePath := filepath.Join(suite.tempFolder, "release", m2dSubFolder, "mirror_000001.tar")

	srcArchive, err := os.Open(archivePath)
	assert.NoError(t, err, "should not fail opening archive after Mirror2Disk")

	defer srcArchive.Close()

	destArchive, err := os.Create(filepath.Join(d2mPath, "mirror_000001.tar"))
	assert.NoError(t, err, "should not fail creating archive file under "+d2mPath)

	defer destArchive.Close()

	_, err = io.Copy(destArchive, srcArchive)
	assert.NoError(t, err, "should not fail copying archive file under "+d2mPath)

}

func TestIntegrationRelease(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite := setupReleaseTest(t)
	defer suite.tearDown()

	suite.runMirror2Disk(t)
	suite.copyArchiveForD2M(t)
	suite.runDisk2Mirror(t)
}

func TestIntegrationReleaseM2M(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite := setupReleaseTest(t)
	defer suite.tearDown()

	suite.runMirror2Mirror(t)
}

func (suite *TestEnvironmentRelease) setupTestData(t *testing.T) {
	t.Setenv("CONTAINERS_REGISTRIES_CONF", suite.tempFolder+"/registries.conf")

	// copy test registries.conf to user home
	regConfTemplatePath := "../../e2e/templates/regisitries.conf"
	err := testutils.FileFromTemplate(suite.tempFolder+"/registries.conf", regConfTemplatePath, []string{suite.sourceRegistryDomain})
	assert.NoError(t, err, "should not fail to prepare registries.conf for test")

	// prepare all images needed
	releaseDigest, releaseImageRefs, err := testutils.GenerateReleaseAndComponents(suite.sourceRegistryDomain, suite.tempFolder, "../../e2e/templates/release_templates/image-references")
	assert.NoError(t, err, "should not fail to generate and push release to source registry")
	suite.releaseImageRefs = releaseImageRefs

	cm := testutils.CincinnatiMock{
		Templates: map[string]string{"stable-4.15": "../../e2e/templates/release_templates/cincinnati_stable-4.15.json"},
		Tokens:    []string{suite.sourceRegistryDomain + "/openshift-release-dev/ocp-release@" + releaseDigest},
	}

	// create a cincinnati endpoint
	ts := httptest.NewServer(http.HandlerFunc(cm.CincinnatiHandler))
	suite.cincinnatiServer = ts
	endpoint, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	suite.cincinnatiEndpoint = endpoint.Host

	// set up a signature in the working-dir (cached signature)
	signatureFile, err := os.Open("../../e2e/signatures/signature-1")
	assert.NoError(t, err)
	defer signatureFile.Close()

	err = os.MkdirAll(suite.tempFolder+"/release/m2d/working-dir/signatures/", 0755)
	assert.NoError(t, err)
	workingDirLocation, err := os.Create(suite.tempFolder + "/release/m2d/working-dir/signatures/" + strings.TrimPrefix(releaseDigest, "sha256:"))
	assert.NoError(t, err)

	defer workingDirLocation.Close()

	_, err = io.Copy(workingDirLocation, signatureFile)
	assert.NoError(t, err)

	graphPrepDir := suite.tempFolder + "/release/m2d/working-dir/graph-preparation"

	err = copy.Copy("../../../tests/graph-staging", graphPrepDir)
	assert.NoError(t, err)

	// create the image set config
	templatePath := "../../e2e/templates/isc_templates/release_isc.yaml"
	suite.imageSetConfig = suite.tempFolder + "/isc.yaml"
	err = testutils.FileFromTemplate(suite.imageSetConfig, templatePath, []string{"stable-4.15"})
	assert.NoError(t, err, "should not fail to generate imageSetConfig")

}

func (suite *TestEnvironmentRelease) runMirror2Disk(t *testing.T) {
	t.Setenv("OC_MIRROR_CACHE", suite.tempFolder+"/.cacheM2D")
	t.Setenv("UPDATE_URL_OVERRIDE", "http://"+suite.cincinnatiEndpoint)

	// create cobra command and run
	ocmirror := NewMirrorCmd(clog.New("trace"))
	// b := bytes.NewBufferString("")
	// ocmirror.SetOut(b)
	resultFolder := filepath.Join(suite.tempFolder, "release", m2dSubFolder)
	err := os.MkdirAll(resultFolder, 0755)
	assert.NoError(t, err, "should not fail creating a temp folder for results")

	ocmirror.SetArgs([]string{"-c", suite.tempFolder + "/isc.yaml", "--v2", "-p", "56001", "--src-tls-verify=false", "--dest-tls-verify=false", "file://" + resultFolder})
	err = ocmirror.Execute()
	assert.NoError(t, err, "should not fail executing oc-mirror")

	// assert results
	assert.FileExists(t, filepath.Join(resultFolder, "mirror_000001.tar"))
}

func (suite *TestEnvironmentRelease) runDisk2Mirror(t *testing.T) {
	t.Setenv("OC_MIRROR_CACHE", suite.tempFolder+"/.cacheD2M")
	t.Setenv("UPDATE_URL_OVERRIDE", "http://"+suite.cincinnatiEndpoint)

	// create cobra command and run
	ocmirror := NewMirrorCmd(clog.New("trace"))
	resultFolder := filepath.Join(suite.tempFolder, "release", d2mSubFolder)

	ocmirror.SetArgs([]string{"-c", suite.tempFolder + "/isc.yaml", "--v2", "-p", "56002", "--from", "file://" + resultFolder, "--src-tls-verify=false", "--dest-tls-verify=false", "docker://" + suite.destinationRegistryDomain + "/release"})
	err := ocmirror.Execute()
	assert.NoError(t, err, "should not fail executing oc-mirror")

	// assert release images exist
	for _, img := range suite.releaseImageRefs {
		destImgRef := strings.Replace(img, suite.sourceRegistryDomain+"/openshift-release-dev/ocp-v4.0-art-dev", suite.destinationRegistryDomain+"/release/openshift/release", -1)
		destImgRef = strings.Replace(destImgRef, suite.sourceRegistryDomain+"/openshift-release-dev/ocp-release", suite.destinationRegistryDomain+"/release/openshift/release-images", -1)
		exists, err := testutils.ImageExists(destImgRef)
		assert.NoError(t, err)
		assert.True(t, exists)
	}

	graphExists, err := testutils.ImageExists(suite.destinationRegistryDomain + "/release/openshift/graph-image:latest")
	assert.NoError(t, err)
	assert.True(t, graphExists)

	// assert IDMS is generated
	assert.FileExists(t, filepath.Join(resultFolder, "working-dir/cluster-resources/idms-oc-mirror.yaml"))
}

func (suite *TestEnvironmentRelease) runMirror2Mirror(t *testing.T) {
	t.Setenv("UPDATE_URL_OVERRIDE", "http://"+suite.cincinnatiEndpoint)
	t.Setenv("OC_MIRROR_CACHE", suite.tempFolder+"/.cacheD2M")

	// create cobra command and run
	ocmirror := NewMirrorCmd(clog.New("trace"))
	// b := bytes.NewBufferString("")
	// ocmirror.SetOut(b)
	resultFolder := filepath.Join(suite.tempFolder, "release", m2dSubFolder)
	err := os.MkdirAll(resultFolder, 0755)
	assert.NoError(t, err, "should not fail creating a temp folder for results")

	ocmirror.SetArgs([]string{"-c", suite.tempFolder + "/isc.yaml", "--v2", "-p", "56003", "--src-tls-verify=false", "--dest-tls-verify=false", "--workspace", "file://" + resultFolder, "docker://" + suite.destinationRegistryDomain + "/release"})
	err = ocmirror.Execute()
	assert.NoError(t, err, "should not fail executing oc-mirror")

	// assert release images exist
	for _, img := range suite.releaseImageRefs {
		destImgRef := strings.Replace(img, suite.sourceRegistryDomain+"/openshift-release-dev/ocp-v4.0-art-dev", suite.destinationRegistryDomain+"/release/openshift/release", -1)
		destImgRef = strings.Replace(destImgRef, suite.sourceRegistryDomain+"/openshift-release-dev/ocp-release", suite.destinationRegistryDomain+"/release/openshift/release-images", -1)

		exists, err := testutils.ImageExists(destImgRef)
		assert.NoError(t, err)
		assert.True(t, exists)
	}

	graphExists, err := testutils.ImageExists(suite.destinationRegistryDomain + "/release/openshift/graph-image:latest")
	assert.NoError(t, err)
	assert.True(t, graphExists)

	// assert IDMS is generated
	assert.FileExists(t, filepath.Join(resultFolder, "working-dir/cluster-resources/idms-oc-mirror.yaml"))
}
