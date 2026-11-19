package consts

const (
	// Protocol constants
	FileProtocol       string = "file://"
	DirProtocol        string = "dir://"
	DockerProtocol     string = "docker://"
	OciProtocol        string = "oci://"
	OciProtocolTrimmed string = "oci:"

	TestFolder string = "../../../tests/unit/testdata/"
	// SignatureFor identifies the signed manifest in a dry-run blob inventory.
	SignatureFor = "io.openshift.oc-mirror.signature-for"
)
