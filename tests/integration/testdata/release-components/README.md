# Release components

The component images of the releases in [`testdata/releases`](../releases/README.md),
which their `image-references` list.

The suite builds an image for every directory here when it starts, and pushes
it to its origin registry as `${ORIGIN_REGISTRY}/oc-mirror/release/<directory>:v0.0.1`.
Nothing is published: to change a component, edit its files and run the tests.

Every directory is an image fixture, described by its `image.yaml` file and its
`rootfs` directory, like the [operands](../operands/README.md). Components are
built for `linux/amd64` as OCI images, or as an OCI index for the components
listed in `multiArchFixtures` (`origin_registry_helpers_test.go`). They are
signed with the test cosign key in `testdata/keys`.

## test-image

The component of `test-release-index`: a script on top of `ubi9-minimal`.

## test-image-multi

The component of `test-release-index-multi`, built for `linux/amd64` and
`linux/arm64`: a script on top of an empty image, labeled as the `alpine`
release component. The release references it by digest.
