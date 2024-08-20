package image

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
		imgSpec, err := ParseRef(imgRef)
		if err != nil {
			t.Errorf("ParseRef() returned unexpected error for %q: %v", imgRef, err)
		}
		actualIsByDigest := imgSpec.IsImageByDigest()

		if actualIsByDigest != expectedisByDigest[i] {
			t.Errorf("isImageByDigest() returned unexpected value for %q: got %v, want %v", imgRef, actualIsByDigest, expectedisByDigest[i])
		}
	}
}

func TestImage_TestParseRef(t *testing.T) {
	type testCase struct {
		caseName        string
		imgRef          string
		expectedImgSpec ImageSpec
		expectedError   string
	}
	testCases := []testCase{
		{
			caseName: "valid docker reference with tag",
			imgRef:   "docker://registry.redhat.io/ubi8/ubi:latest",
			expectedImgSpec: ImageSpec{
				Transport:              "docker://",
				Reference:              "registry.redhat.io/ubi8/ubi:latest",
				ReferenceWithTransport: "docker://registry.redhat.io/ubi8/ubi:latest",
				Name:                   "registry.redhat.io/ubi8/ubi",
				Domain:                 "registry.redhat.io",
				PathComponent:          "ubi8/ubi",
				Tag:                    "latest",
				Digest:                 "",
			},
			expectedError: "",
		},
		{
			caseName: "valid docker reference with tag and digest",
			imgRef:   "docker://registry.redhat.io/ubi8/ubi:latest@sha256:44d75007b39e0e1bbf1bcfd0721245add54c54c3f83903f8926fb4bef6827aa2",
			expectedImgSpec: ImageSpec{
				Transport:              "docker://",
				Reference:              "registry.redhat.io/ubi8/ubi:latest@sha256:44d75007b39e0e1bbf1bcfd0721245add54c54c3f83903f8926fb4bef6827aa2",
				ReferenceWithTransport: "docker://registry.redhat.io/ubi8/ubi:latest@sha256:44d75007b39e0e1bbf1bcfd0721245add54c54c3f83903f8926fb4bef6827aa2",
				Name:                   "registry.redhat.io/ubi8/ubi",
				Domain:                 "registry.redhat.io",
				PathComponent:          "ubi8/ubi",
				Tag:                    "latest",
				Algorithm:              "sha256",
				Digest:                 "44d75007b39e0e1bbf1bcfd0721245add54c54c3f83903f8926fb4bef6827aa2",
			},
			expectedError: "",
		},
		{
			caseName: "valid docker reference with digest",
			imgRef:   "docker://registry.redhat.io/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
			expectedImgSpec: ImageSpec{
				Transport:              "docker://",
				Reference:              "registry.redhat.io/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
				ReferenceWithTransport: "docker://registry.redhat.io/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
				Name:                   "registry.redhat.io/ubi8/ubi",
				Domain:                 "registry.redhat.io",
				PathComponent:          "ubi8/ubi",
				Tag:                    "",
				Algorithm:              "sha256",
				Digest:                 "db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
			},
			expectedError: "",
		},
		{
			caseName: "valid docker reference on local registry with tag",
			imgRef:   "docker://localhost:5000/ubi8/ubi:latest",
			expectedImgSpec: ImageSpec{
				Transport:              "docker://",
				Reference:              "localhost:5000/ubi8/ubi:latest",
				ReferenceWithTransport: "docker://localhost:5000/ubi8/ubi:latest",
				Name:                   "localhost:5000/ubi8/ubi",
				Domain:                 "localhost:5000",
				PathComponent:          "ubi8/ubi",
				Tag:                    "latest",
				Digest:                 "",
			},
			expectedError: "",
		},
		{
			caseName: "valid docker reference on local registry with tag and digest",
			imgRef:   "docker://localhost:5000/ubi8/ubi:abcd@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
			expectedImgSpec: ImageSpec{
				Transport:              "docker://",
				Reference:              "localhost:5000/ubi8/ubi:abcd@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
				ReferenceWithTransport: "docker://localhost:5000/ubi8/ubi:abcd@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
				Name:                   "localhost:5000/ubi8/ubi",
				Domain:                 "localhost:5000",
				PathComponent:          "ubi8/ubi",
				Tag:                    "abcd",
				Algorithm:              "sha256",
				Digest:                 "db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
			},
			expectedError: "",
		},
		{
			caseName: "valid docker reference on local registry with digest",
			imgRef:   "docker://localhost:5000/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
			expectedImgSpec: ImageSpec{
				Transport:              "docker://",
				Reference:              "localhost:5000/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
				ReferenceWithTransport: "docker://localhost:5000/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
				Name:                   "localhost:5000/ubi8/ubi",
				Domain:                 "localhost:5000",
				PathComponent:          "ubi8/ubi",
				Tag:                    "",
				Digest:                 "db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
				Algorithm:              "sha256",
			},
			expectedError: "",
		},
		{
			caseName: "valid docker reference on local registry with tag no transport",
			imgRef:   "localhost:5000/ubi8/ubi:latest",
			expectedImgSpec: ImageSpec{
				Transport:              "docker://",
				Reference:              "localhost:5000/ubi8/ubi:latest",
				ReferenceWithTransport: "docker://localhost:5000/ubi8/ubi:latest",
				Name:                   "localhost:5000/ubi8/ubi",
				Domain:                 "localhost:5000",
				PathComponent:          "ubi8/ubi",
				Tag:                    "latest",
				Digest:                 "",
			},
			expectedError: "",
		},
		{
			caseName: "valid docker reference on local registry with digest no transport",
			imgRef:   "localhost:5000/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
			expectedImgSpec: ImageSpec{
				Transport:              "docker://",
				Reference:              "localhost:5000/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
				ReferenceWithTransport: "docker://localhost:5000/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
				Name:                   "localhost:5000/ubi8/ubi",
				Domain:                 "localhost:5000",
				PathComponent:          "ubi8/ubi",
				Tag:                    "",
				Digest:                 "db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
				Algorithm:              "sha256",
			},
			expectedError: "",
		},
		{
			caseName: "valid docker reference with tag no transport",
			imgRef:   "registry.redhat.io/ubi8/ubi:latest",
			expectedImgSpec: ImageSpec{
				Transport:              "docker://",
				Reference:              "registry.redhat.io/ubi8/ubi:latest",
				ReferenceWithTransport: "docker://registry.redhat.io/ubi8/ubi:latest",
				Name:                   "registry.redhat.io/ubi8/ubi",
				Domain:                 "registry.redhat.io",
				PathComponent:          "ubi8/ubi",
				Tag:                    "latest",
				Digest:                 "",
			},
			expectedError: "",
		},
		{
			caseName: "valid docker reference with digest no transport",
			imgRef:   "registry.redhat.io/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
			expectedImgSpec: ImageSpec{
				Transport:              "docker://",
				Reference:              "registry.redhat.io/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
				ReferenceWithTransport: "docker://registry.redhat.io/ubi8/ubi@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
				Name:                   "registry.redhat.io/ubi8/ubi",
				Domain:                 "registry.redhat.io",
				PathComponent:          "ubi8/ubi",
				Tag:                    "",
				Digest:                 "db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
				Algorithm:              "sha256",
			},
			expectedError: "",
		},
		{
			caseName: "valid oci reference full path",
			imgRef:   "oci:///tmp/ubi8/ubi",
			expectedImgSpec: ImageSpec{
				Transport:              "oci://",
				Reference:              "/tmp/ubi8/ubi",
				ReferenceWithTransport: "oci:///tmp/ubi8/ubi",
				Name:                   "/tmp/ubi8/ubi",
				Domain:                 "",
				PathComponent:          "/tmp/ubi8/ubi",
				Tag:                    "",
				Digest:                 "",
			},
			expectedError: "",
		},
		{
			caseName: "valid oci reference relative path",
			imgRef:   "oci://ubi8/ubi",
			expectedImgSpec: ImageSpec{
				Transport:              "oci://",
				Reference:              "ubi8/ubi",
				ReferenceWithTransport: "oci://ubi8/ubi",
				Name:                   "ubi8/ubi",
				Domain:                 "",
				PathComponent:          "ubi8/ubi",
				Tag:                    "",
				Digest:                 "",
			},
			expectedError: "",
		},
		{
			caseName:        "valid docker reference implicit domain succeeds",
			imgRef:          "abcde:latest",
			expectedImgSpec: ImageSpec{Transport: "docker://", Reference: "abcde:latest", ReferenceWithTransport: "docker://abcde:latest", Name: "abcde", Domain: "", PathComponent: "abcde", Tag: "latest", Digest: ""},
			expectedError:   "",
		},
		{
			caseName:        "invalid docker reference fails",
			imgRef:          "whatever",
			expectedImgSpec: ImageSpec{},
			expectedError:   "whatever unable to parse image correctly : tag and digest are empty",
		},
	}

	for _, aTestCase := range testCases {
		t.Run(aTestCase.caseName, func(t *testing.T) {
			imgSpec, err := ParseRef(aTestCase.imgRef)
			if aTestCase.expectedError != "" && err == nil {
				t.Errorf("ParseRef() expected to fail for %q: got %v, want %v", aTestCase.imgRef, err, aTestCase.expectedError)
			}
			if err != nil {
				if aTestCase.expectedError != err.Error() {
					t.Errorf("ParseRef() returned unexpected error for %q: got %v, want %v", aTestCase.imgRef, err, aTestCase.expectedError)
				}
			} else {
				require.Equal(t, aTestCase.expectedImgSpec, imgSpec)

			}
		})
	}
}

func TestImage_TestWithMaxNestedPaths(t *testing.T) {
	type testCase struct {
		caseName       string
		imgRef         string
		maxNestedPaths int
		expectedOutRef string
		expectedError  string
	}
	testCases := []testCase{
		{
			caseName:       "WithMaxNestedPaths(2) : should pass",
			imgRef:         "myregistry.mydomain/ocpbugs-11922/mirror-release/gitlab-org/ci-cd/gitlab-runner-ubi-images/gitlab-runner-helper-ocp@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
			maxNestedPaths: 2,
			expectedOutRef: "myregistry.mydomain/ocpbugs-11922/mirror-release-gitlab-org-ci-cd-gitlab-runner-ubi-images-gitlab-runner-helper-ocp@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
			expectedError:  "",
		},
		{
			caseName:       "WithMaxNestedPaths(3) : should pass",
			imgRef:         "myregistry.mydomain/ocpbugs-11922/mirror-release/gitlab-org/ci-cd/gitlab-runner-ubi-images/gitlab-runner-helper-ocp@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
			maxNestedPaths: 3,
			expectedOutRef: "myregistry.mydomain/ocpbugs-11922/mirror-release/gitlab-org-ci-cd-gitlab-runner-ubi-images-gitlab-runner-helper-ocp@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
			expectedError:  "",
		},
		{
			caseName:       "WithMaxNestedPaths(4) : should pass",
			imgRef:         "myregistry.mydomain/ocpbugs-11922/mirror-release/gitlab-org/ci-cd/gitlab-runner-ubi-images/gitlab-runner-helper-ocp@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
			maxNestedPaths: 4,
			expectedOutRef: "myregistry.mydomain/ocpbugs-11922/mirror-release/gitlab-org/ci-cd-gitlab-runner-ubi-images-gitlab-runner-helper-ocp@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
			expectedError:  "",
		},
		{
			caseName:       "WithMaxNestedPaths(5) : should pass",
			imgRef:         "myregistry.mydomain/ocpbugs-11922/mirror-release/gitlab-org/ci-cd/gitlab-runner-ubi-images/gitlab-runner-helper-ocp@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
			maxNestedPaths: 5,
			expectedOutRef: "myregistry.mydomain/ocpbugs-11922/mirror-release/gitlab-org/ci-cd/gitlab-runner-ubi-images-gitlab-runner-helper-ocp@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
			expectedError:  "",
		},
		{
			caseName:       "WithMaxNestedPaths(6) : should pass",
			imgRef:         "myregistry.mydomain/ocpbugs-11922/mirror-release/gitlab-org/ci-cd/gitlab-runner-ubi-images/gitlab-runner-helper-ocp@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
			maxNestedPaths: 6,
			expectedOutRef: "myregistry.mydomain/ocpbugs-11922/mirror-release/gitlab-org/ci-cd/gitlab-runner-ubi-images/gitlab-runner-helper-ocp@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
			expectedError:  "",
		},
		{
			caseName:       "WithMaxNestedPaths(0) : should pass",
			imgRef:         "myregistry.mydomain/ocpbugs-11922/mirror-release/gitlab-org/ci-cd/gitlab-runner-ubi-images/gitlab-runner-helper-ocp@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
			maxNestedPaths: 0,
			expectedOutRef: "myregistry.mydomain/ocpbugs-11922/mirror-release/gitlab-org/ci-cd/gitlab-runner-ubi-images/gitlab-runner-helper-ocp@sha256:db870970ba330193164dacc88657df261d75bce1552ea474dbc7cf08b2fae2ed",
			expectedError:  "",
		},
		{
			caseName:       "WithMaxNestedPaths(2) invalid ref : should fail",
			imgRef:         "whatever",
			maxNestedPaths: 2,
			expectedOutRef: "",
			expectedError:  "whatever unable to parse image correctly : tag and digest are empty",
		},
	}
	for _, aTestCase := range testCases {
		t.Run(aTestCase.caseName, func(t *testing.T) {
			outRef, err := WithMaxNestedPaths(aTestCase.imgRef, aTestCase.maxNestedPaths)
			if aTestCase.expectedError != "" && err == nil {
				t.Errorf("ParseRef() expected to fail for %q: got %v, want %v", aTestCase.imgRef, err, aTestCase.expectedError)
			}
			if err != nil {
				if aTestCase.expectedError != err.Error() {
					t.Errorf("ParseRef() returned unexpected error for %q: got %v, want %v", aTestCase.imgRef, err, aTestCase.expectedError)
				}
			} else {
				require.Equal(t, aTestCase.expectedOutRef, outRef)
			}
		})
	}
}
