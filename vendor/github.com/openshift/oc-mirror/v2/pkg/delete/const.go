package delete

const (
	deleteDir               string = "/delete"
	deleteImagesYaml        string = "delete/delete-images.yaml"
	discYaml                string = "delete/delete-imageset-config.yaml"
	dockerProtocol          string = "docker://"
	operatorImageExtractDir string = "hold-operator"
	ociProtocol             string = "oci://"
	ociProtocolTrimmed      string = "oci:"
	operatorImageDir        string = "operator-images"
	blobsDir                string = "docker/registry/v2/blobs/sha256"
	releaseManifests        string = "release-manifests"
	imageReferences         string = "image-references"
	deleteImagesErrMsg      string = "[delete-images] %v"
)
