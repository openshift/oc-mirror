# multilayers

OCI image with more than 10 layers, used by integration tests for `--parallel-layers` behavior.

## Build

```bash
export IMAGE_VERSION="$(date -u +%Y%m%d)-$(git rev-parse --short HEAD)"
make build
```

## Publish (maintainer only)

Push to `quay.io/oc-mirror/oc-mirror-dev` (not `openshifttest`):

```bash
make push
```

The image is published as `quay.io/oc-mirror/oc-mirror-dev:multilayers-${IMAGE_VERSION}`.
Use the same exported version for build and push, and never overwrite a published
tag. Update the affected ISCs and Go expectations to adopt a new image.

The integration tests currently pin
`quay.io/oc-mirror/oc-mirror-dev:multilayers-20261103-2e526488d`.
See the [image versioning guide](../../README.md) for publishing new fixture versions.
