# Operands

Images that the bundles of [`testdata/bundles`](../bundles/README.md) refer to,
like the operator and operand images of real bundles.

The suite builds an image for every directory here when it starts, and pushes
it to its origin registry as `${ORIGIN_REGISTRY}/oc-mirror/oc-mirror-dev:<directory>`.
Nothing is published: to change an operand, edit its files and run the tests.

Every directory is an image fixture, like the
[release components](../release-components/README.md):

* `image.yaml` describes the image: an optional base image, pinned by digest,
  and the configuration set on top of it. The configuration has the fields of
  the `config` of an OCI image configuration (`Entrypoint`, `Cmd`, `Env`,
  `User`, `WorkingDir` and `Labels`), which are set like the instructions of a
  Dockerfile.
* `rootfs` holds the files added on top of the base image, at the root of the
  image.

For example:

```yaml
base: gcr.io/distroless/static@sha256:ef577529a4e1591d0e56ad7d03e0a7341d6da92f2da72ed2a1ea6f1440887ed4
config:
  Entrypoint: [/run.sh]
```

Operands are built for `linux/amd64` as Docker schema 2 images, or as an OCI
index for the operands listed in `multiArchFixtures`
(`origin_registry_helpers_test.go`). They are signed with the test cosign key in
`testdata/keys`.

## operand

Every bundle refers to it by digest. Like the related images published before,
it is `run.sh` on top of `gcr.io/distroless/static:debug`.
