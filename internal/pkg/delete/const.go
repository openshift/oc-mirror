package delete

const (
	deleteDir                   string = "/delete"
	deleteImagesYaml            string = "delete/delete-images.yaml"
	discYaml                    string = "delete/delete-imageset-config.yaml"
	dockerProtocol              string = "docker://"
	operatorImageExtractDir     string = "hold-operator"
	ociProtocol                 string = "oci://"
	ociProtocolTrimmed          string = "oci:"
	operatorImageDir            string = "operator-images"
	blobsDir                    string = "docker/registry/v2/blobs/sha256"
	releaseManifests            string = "release-manifests"
	imageReferences             string = "image-references"
	deleteImagesErrMsg          string = "[delete-images] %v"
	releaseImageExtractFullPath string = releaseManifests + "/" + imageReferences
	releaseImageExtractDir      string = "hold-release"
	ocpRelease                  string = "ocp-release"
	errMsg                      string = "[ReleaseImageCollector] %v "
	logFile                     string = "release.log"
	x86_64                      string = "x86_64"
	amd64                       string = "x86_64"
	s390x                       string = "s390x"
	ppc64le                     string = "ppc64le"
	aarch64                     string = "aarch64"
	arm64                       string = "aarch64"
	multi                       string = "multi"
	releaseRepo                 string = "docker://quay.io/openshift-release-dev/ocp-release"
)
