# Releases

Mock OpenShift release payloads to be used in tests.

The suite builds an image for every directory here when it starts, and pushes it
to its origin registry as `${ORIGIN_REGISTRY}/oc-mirror/release/<directory>:v0.0.1`,
which is how ImageSetConfigurations reference it. Nothing is published: to change
a release, edit its files and run the tests.

Like an OpenShift release image, the only layer of a payload holds its
`release-manifests` directory:

* `image-references` lists the release component images, built from
  [`testdata/release-components`](../release-components/README.md). It refers
  to them with the placeholders described in
  [`testdata/catalogs`](../catalogs/README.md).
* `release-metadata` holds the release version, also set as the
  `io.openshift.release` label.

Releases are built as a `linux/amd64` image. The releases listed in
`multiArchFixtures` (`origin_registry_helpers_test.go`) are built as an OCI
index instead, with one image per platform.

The payloads are signed with the test cosign key in `testdata/keys`. oc-mirror
also verifies the GPG signature of a release, and pulls it from the
`docker-reference` of that signature. So the suite signs each release with a new
GPG key, and sets `OCP_SIGNATURE_VERIFICATION_PK` to its public key.

## test-release-index

A `linux/amd64` payload with one component, `test-image`, referenced by tag.

## test-release-index-multi

A payload for `linux/amd64`, `linux/arm64`, `linux/s390x` and `linux/ppc64le`, for
the sparse manifest tests. Its component, `test-image-multi`, is an image index
referenced by digest.
