# AI Agent Guide for oc-mirror

This file provides guidance to AI agents when working with code in this repository.

## Project Overview

This is the `oc-mirror` repository - an OpenShift client tool for mirroring container registry content for disconnected cluster installs.
oc-mirror reads an `ImageSetConfiguration` YAML file and mirrors container images from source registries to:
- a target mirror registry (direct mirroring)
- a local cache on disk, then generates a tarball for later mirroring (air-gapped scenarios)

### Key workflows
1. **mirrorToMirror (m2m)** : direct registry-to-registry copying
1. **mirrorToDisk (m2d)** : copy images to local cache and create tarball
1. **diskToMirror (d2m)** : copy images from tarball to target registry

### Version structure
- **v2** (Current) : code lives in the root directory - **THIS IS WHAT YOU SHOULD WORK ON**
- **v1** (Deprecated) : code lives under the v1/ folder - **DO NOT MODIFY v1 CODE**

## Key Architecture components

The `oc-mirror` project relies heavily on the [container-libs](https://github.com/containers/container-libs) library for low-level container image operations.

For each container image type, `oc-mirror` defines a `Collector` interface responsible for discovering all the related images:

| Collector | Location | Purpose |
|-----------|----------|---------|
| Release | `internal/pkg/release` | Openshift release payloads |
| Operator | `internal/pkg/operator`| RedHat operator catalogs |
| Helm | `internal/pkg/helm`| Helm charts |
| Additional | `internal/pkg/additional` | generic container images |

The image copying itself happens concurrently in batches via `internal/pkg/batch` for optimal performance.

The `ImageSetConfiguration` is defined in `internal/pkg/api/v2alpha1/`. See `docs/image-set-examples/imaget-set-config.yaml` for examples.

## Common development commands

### Building

```bash
make clean # clean up build artifacts
make build # compiles the oc-mirror binary
```

**Important**: always use `make build`, not `go build` directly - the Makefile sets required build tags.

### Testing

```bash
make test-unit        # run unit tests
make test-integration # run integration tests
```

### Validation and Verification

```bash
make verify # run golangci-lint
make sanity # runs: tidy, format, and vet checks
```

## Contributing

1. Write understandable code. Always prefer clarity over other things.
1. Write comments and documentation in English.
1. Write unit tests for your code.
1. When instructed to fix tests, do not remove or modify existing tests.
1. Write documentation for your code.
1. Run `make sanity` before committing files.
