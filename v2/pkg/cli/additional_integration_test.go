package cli

import (
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openshift/oc-mirror/v2/internal/testutils"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
	"github.com/stretchr/testify/assert"
)

const (
	d2mSubFolder string = "d2m"
	m2dSubFolder string = "m2d"
	m2mSubFolder string = "m2m"
)

type TestEnvironmentAddditional struct {
	sourceServer              *httptest.Server
	destinationServer         *httptest.Server
	sourceRegistryDomain      string
	destinationRegistryDomain string
	tempFolder                string
	imageSetConfig            string
	additionalImageRefs       []string
}

// before all the tests
func setupAdditionalTest(t *testing.T) TestEnvironmentAddditional {
	suite := TestEnvironmentAddditional{}
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

func (suite *TestEnvironmentAddditional) tearDown(t *testing.T) {
	suite.sourceServer.Close()
	suite.destinationServer.Close()
	os.RemoveAll(suite.tempFolder)
}

func (suite *TestEnvironmentAddditional) copyArchiveForD2M(t *testing.T) {
	d2mPath := filepath.Join(suite.tempFolder, "additional", d2mSubFolder)
	err := os.MkdirAll(d2mPath, 0755)
	assert.NoError(t, err, "should not fail creating "+d2mPath)
	archivePath := filepath.Join(suite.tempFolder, "additional", m2dSubFolder, "mirror_000001.tar")

	srcArchive, err := os.Open(archivePath)
	assert.NoError(t, err, "should not fail opening archive after Mirror2Disk")

	defer srcArchive.Close()

	destArchive, err := os.Create(filepath.Join(d2mPath, "mirror_000001.tar"))
	assert.NoError(t, err, "should not fail creating archive file under "+d2mPath)

	defer destArchive.Close()

	_, err = io.Copy(destArchive, srcArchive)
	assert.NoError(t, err, "should not fail copying archive file under "+d2mPath)

}
func TestIntegrationAdditional(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite := setupAdditionalTest(t)
	defer suite.tearDown(t)

	suite.runMirror2Disk(t)
	suite.copyArchiveForD2M(t)
	suite.runDisk2Mirror(t)

}
func TestIntegrationAdditionalM2M(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite := setupAdditionalTest(t)
	defer suite.tearDown(t)

	suite.runMirror2Mirror(t)

}

func (suite *TestEnvironmentAddditional) setupTestData(t *testing.T) {

	// prepare all images needed
	suite.additionalImageRefs = []string{}
	suite.additionalImageRefs = append(suite.additionalImageRefs, suite.sourceRegistryDomain+"/foo:v1.0")
	_, err := testutils.GenerateFakeImage("additional", suite.additionalImageRefs[0], suite.tempFolder)
	assert.NoError(t, err, "should not fail to push image"+suite.sourceRegistryDomain+"/foo:v1.0")

	// create the image set config
	templatePath := "../../internal/e2e/templates/isc_templates/additional_isc.yaml"
	suite.imageSetConfig = suite.tempFolder + "/isc.yaml"
	err = testutils.FileFromTemplate(suite.imageSetConfig, templatePath, suite.additionalImageRefs)
	assert.NoError(t, err, "should not fail to generate imageSetConfig")

}

func (suite *TestEnvironmentAddditional) runMirror2Disk(t *testing.T) {

	// create cobra command and run
	ocmirror := NewMirrorCmd(clog.New("trace"))
	// b := bytes.NewBufferString("")
	// ocmirror.SetOut(b)
	resultFolder := filepath.Join(suite.tempFolder, "additional", m2dSubFolder)
	err := os.MkdirAll(resultFolder, 0755)
	assert.NoError(t, err, "should not fail creating a temp folder for results")

	os.Setenv("OC_MIRROR_CACHE", suite.tempFolder+"/.cacheM2D")
	ocmirror.SetArgs([]string{"-c", suite.tempFolder + "/isc.yaml", "--v2", "-p", "55001", "--src-tls-verify=false", "--dest-tls-verify=false", "file://" + resultFolder})
	err = ocmirror.Execute()
	assert.NoError(t, err, "should not fail executing oc-mirror")

	// assert results
	assert.FileExists(t, filepath.Join(resultFolder, "mirror_000001.tar"))
}

func (suite *TestEnvironmentAddditional) runDisk2Mirror(t *testing.T) {
	ocmirror := NewMirrorCmd(clog.New("trace"))
	resultFolder := filepath.Join(suite.tempFolder, "additional", d2mSubFolder)
	os.Setenv("OC_MIRROR_CACHE", suite.tempFolder+"/.cacheD2M")
	ocmirror.SetArgs([]string{"-c", suite.tempFolder + "/isc.yaml", "--v2", "-p", "55002", "--from", "file://" + resultFolder, "--src-tls-verify=false", "--dest-tls-verify=false", "docker://" + suite.destinationRegistryDomain + "/additional"})
	err := ocmirror.Execute()
	assert.NoError(t, err, "should not fail executing oc-mirror")

	// assert additional image exists
	destImgRef := strings.Replace(suite.additionalImageRefs[0], suite.sourceRegistryDomain, suite.destinationRegistryDomain+"/additional", -1)
	exists, err := testutils.ImageExists(destImgRef)
	assert.NoError(t, err)
	assert.True(t, exists)

	// assert ITMS is generated
	assert.FileExists(t, filepath.Join(suite.tempFolder, "additional", d2mSubFolder, "/working-dir/cluster-resources/itms-oc-mirror.yaml"))

}

func (suite *TestEnvironmentAddditional) runMirror2Mirror(t *testing.T) {
	os.Setenv("OC_MIRROR_CACHE", suite.tempFolder+"/.cacheM2D")
	// create cobra command and run
	ocmirror := NewMirrorCmd(clog.New("trace"))
	// b := bytes.NewBufferString("")
	// ocmirror.SetOut(b)
	resultFolder := filepath.Join(suite.tempFolder, "additional", m2mSubFolder)
	err := os.MkdirAll(resultFolder, 0755)
	assert.NoError(t, err, "should not fail creating a temp folder for results")

	ocmirror.SetArgs([]string{"-c", suite.tempFolder + "/isc.yaml", "--v2", "-p", "55003", "--src-tls-verify=false", "--dest-tls-verify=false", "--workspace", "file://" + resultFolder, "docker://" + suite.destinationRegistryDomain + "/additional"})
	err = ocmirror.Execute()
	assert.NoError(t, err, "should not fail executing oc-mirror")

	// assert additional image exists
	destImgRef := strings.Replace(suite.additionalImageRefs[0], suite.sourceRegistryDomain, suite.destinationRegistryDomain+"/additional", -1)
	exists, err := testutils.ImageExists(destImgRef)
	assert.NoError(t, err)
	assert.True(t, exists)

	// assert ITMS is generated
	assert.FileExists(t, filepath.Join(suite.tempFolder, "additional", m2mSubFolder, "/working-dir/cluster-resources/itms-oc-mirror.yaml"))

}
