# multilayers

OCI image with more than 10 layers, used by integration tests for `--parallel-layers` behavior.

## Build

```bash
make build
```

## Publish (maintainer only)

Push to `quay.io/oc-mirror/oc-mirror-dev` (not `openshifttest`):

```bash
make push
```

The integration test ISC references `quay.io/oc-mirror/oc-mirror-dev:multilayers-latest`.
