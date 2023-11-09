package archive

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStoreBlobGatherer_imagePathComponents(t *testing.T) {
	imgRefs := []string{
		"docker://localhost:5000/ubi8/ubi:latest",
		"docker://localhost:5000/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
		"docker://registry.redhat.io/ubi8/ubi:latest",
		"docker://registry.redhat.io/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
		"localhost:5000/ubi8/ubi:latest",
		"localhost:5000/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
		"file:///tmp/ubi8/ubi",
		"oci:///tmp/ubi8/ubi",
	}
	expectedPathComponents := []string{
		"ubi8/ubi",
		"ubi8/ubi",
		"ubi8/ubi",
		"ubi8/ubi",
		"ubi8/ubi",
		"ubi8/ubi",
		"tmp/ubi8/ubi",
		"tmp/ubi8/ubi",
	}

	for i, imgRef := range imgRefs {
		actualPathComponents, err := imagePathComponents(imgRef)
		if err != nil {
			t.Fatal(err)
		}
		if actualPathComponents != expectedPathComponents[i] {
			t.Errorf("imagePathComponents() returned unexpected value for %q: got %q, want %q", imgRef, actualPathComponents, expectedPathComponents[i])
		}
	}
}

func TestStoreBlobGatherer_isImageByDigest(t *testing.T) {
	imgRefs := []string{
		"docker://localhost:5000/ubi8/ubi:latest",
		"docker://localhost:5000/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
	}
	expectedisByDigest := []bool{
		false,
		true,
	}

	for i, imgRef := range imgRefs {
		actualIsByDigest := isImageByDigest(imgRef)

		if actualIsByDigest != expectedisByDigest[i] {
			t.Errorf("isImageByDigest() returned unexpected value for %q: got %v, want %v", imgRef, actualIsByDigest, expectedisByDigest[i])
		}
	}
}

func TestStoreBlobGatherer_GatherBlobs(t *testing.T) {

	// Create a new StoreBlobGatherer with the temporary directory as the local storage location
	gatherer := NewStoreBlobGatherer("../../tests/cache-fake")

	// Call GatherBlobs with a test image reference
	actualBlobs, err := gatherer.GatherBlobs("docker://localhost:5000/ubi8/ubi:latest")
	if err != nil {
		t.Fatal(err)
	}

	// Check that the returned blobs map contains the expected key-value pair
	expectedBlobs := map[string]string{
		"db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed": "",
		"e6c589cf5f402a60a83a01653304d7a8dcdd47b93a395a797b5622a18904bd66": "",
		"9b6fa335dba394d437930ad79e308e01da4f624328e49d00c0ff44775d2e4769": "",
		"6376a0276facf61d87fdf7c6f21d761ee25ba8ceba934d64752d43e84fe0cb98": "",
		"2e39d55595ea56337b5b788e96e6afdec3db09d2759d903cbe120468187c4644": "",
		"4c0f6aace7053de3b9c1476b33c9a763e45a099c8c7ae9117773c9a8e5b8506b": "",
		"53c56977ccd20c0d87df0ad52036c55b27201e1a63874c2644383d0c532f5aee": "",
		"6e1ac33d11e06db5e850fec4a1ec07f6c2ab15f130c2fdf0f9d0d0a5c83651e7": "",
		"94343313ec1512ab02267e4bc3ce09eecb01fda5bf26c56e2f028ecc72e80b18": "",
		"cfaa7496ab546c36ab14859f93fbd2d8a3588b344b18d5fbe74dd834e4a6f7eb": "",
		"e1bb0572465a9e03d7af5024abb36d7227b5bf133c448b54656d908982127874": "",
		"f992cb38fce665360a4d07f6f78db864a1f6e20a7ad304219f7f81d7fe608d97": "",
	}
	assert.Equal(t, expectedBlobs, actualBlobs)
}
