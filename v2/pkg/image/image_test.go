package image

import "testing"

func TestImage_IsImageByDigest(t *testing.T) {
	imgRefs := []string{
		"docker://localhost:5000/ubi8/ubi:latest",
		"docker://localhost:5000/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
	}
	expectedisByDigest := []bool{
		false,
		true,
	}

	for i, imgRef := range imgRefs {
		actualIsByDigest := IsImageByDigest(imgRef)

		if actualIsByDigest != expectedisByDigest[i] {
			t.Errorf("isImageByDigest() returned unexpected value for %q: got %v, want %v", imgRef, actualIsByDigest, expectedisByDigest[i])
		}
	}
}

func TestImage_RefWithoutTrasport(t *testing.T) {
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
		"localhost:5000/ubi8/ubi:latest",
		"localhost:5000/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
		"registry.redhat.io/ubi8/ubi:latest",
		"registry.redhat.io/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
		"localhost:5000/ubi8/ubi:latest",
		"localhost:5000/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
		"/tmp/ubi8/ubi",
		"/tmp/ubi8/ubi",
	}

	for i, imgRef := range imgRefs {
		actualPathComponents := RefWithoutTransport(imgRef)

		if actualPathComponents != expectedPathComponents[i] {
			t.Errorf("image.RefWithoutTransport() returned unexpected value for %q: got %q, want %q", imgRef, actualPathComponents, expectedPathComponents[i])
		}
	}
}

func TestPathWithoutDNS(t *testing.T) {
	imgRefs := []string{
		"docker://localhost:5000/ubi8/ubi:latest",
		"docker://registry.redhat.io/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
		"registry.redhat.io/ubi8/ubi:latest",
		"abcde:latest",
		"localhost:5000/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
		"folderName",
		"oci:///tmp/ubi8/ubi",
	}
	expectedPaths := []string{
		"",
		"",
		"ubi8/ubi:latest",
		"abcde:latest",
		"ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
		"folderName",
		"",
	}
	expectedErrors := []string{
		"image reference should not contain transport prefix",
		"image reference should not contain transport prefix",
		"",
		"",
		"",
		"",
		"image reference should not contain transport prefix",
	}

	for i, imgRef := range imgRefs {
		actualPath, err := PathWithoutDNS(imgRef)

		if err != nil && expectedErrors[i] == "" {
			t.Errorf("PathWithoutDNS() returned an unexpected error for %q: %v", imgRef, err)
		}

		if err == nil && expectedErrors[i] != "" {
			t.Errorf("PathWithoutDNS() was expected to return an error for %q: got nil, want %q", imgRef, expectedErrors[i])
		}

		if err != nil && err.Error() != expectedErrors[i] {
			t.Errorf("PathWithoutDNS() returned unexpected error for %q: got %q, want %q", imgRef, err.Error(), expectedErrors[i])
		}

		if actualPath != expectedPaths[i] {
			t.Errorf("PathWithoutDNS() returned unexpected value for %q: got %q, want %q", imgRef, actualPath, expectedPaths[i])
		}
	}
}
