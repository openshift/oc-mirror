# AI Agent Guide for oc-mirror

This file provides guidance to AI agents when working with code in this repository.

## Project Overview

This is the `oc-mirror` repository — an OpenShift client tool for mirroring container registry content for disconnected cluster installs.
oc-mirror reads an `ImageSetConfiguration` YAML file and mirrors container images from source registries to:
- a target mirror registry (direct mirroring)
- a local cache on disk, then generates a tarball for later mirroring (air-gapped scenarios)

### Key workflows
1. **mirrorToMirror (m2m)** : direct registry-to-registry copying
1. **mirrorToDisk (m2d)** : copy images to local cache and create tarball
1. **diskToMirror (d2m)** : copy images from tarball to target registry

### Version structure
- **v2** (Current) : code lives in the root directory — **THIS IS WHAT YOU SHOULD WORK ON**
- **v1** (Deprecated) : code lives under the `v1/` folder — **DO NOT MODIFY v1 CODE**

## Project structure

```
oc-mirror/
├── cmd/oc-mirror/          # CLI entrypoint and embedded data
├── pkg/
│   ├── cli/mirror/         # top-level mirror orchestration (m2m, m2d, d2m)
│   └── metadata/storage/   # metadata/history persistence
├── internal/
│   ├── pkg/
│   │   ├── api/v2alpha1/   # ImageSetConfiguration types and schemas
│   │   ├── release/        # OpenShift release payload collector
│   │   ├── operator/       # Operator catalog collector (+ filtering)
│   │   ├── helm/           # Helm chart collector
│   │   ├── additional/     # Generic additional image collector
│   │   ├── batch/          # Concurrent image copy worker
│   │   ├── archive/        # Tarball creation/extraction
│   │   ├── mirror/         # Low-level copy interfaces
│   │   ├── image/          # Image reference parsing
│   │   ├── clusterresources/ # IDMS/ITMS/CatalogSource generation
│   │   ├── cincinnati/     # Cincinnati/update graph client
│   │   ├── signature/      # Image signature handling
│   │   ├── delete/         # Image deletion logic
│   │   ├── history/        # Mirror run history tracking
│   │   ├── cli/            # Internal CLI executor
│   │   ├── config/         # Registry auth and config
│   │   └── ...             # log, emoji, spinners, consts, etc.
│   ├── e2e/                # E2E test infrastructure and templates
│   └── testutils/          # Shared test helpers
├── tests/                  # Test fixtures (OCI images, configs, caches)
├── test/e2e/               # E2E test data
├── docs/                   # Documentation (see below)
├── hack/                   # Build and CI scripts
├── images/cli/             # Container image build (Dockerfile support)
└── v1/                     # Deprecated v1 code — DO NOT MODIFY
```

### Documentation tree

```
docs/
├── dev-investigations/
│   ├── OCPSTRAT-1515.md                              # strategy investigation
│   ├── operator-filtering-investigation.md            # operator filtering design
│   └── progress-bar-and-concurrencty-investigation.md # progress/concurrency design
├── features/
│   ├── delete-functionality.md       # image deletion feature spec
│   ├── enclave_support.md            # enclave/air-gap support
│   └── signature-verification.md     # signature verification design
├── image-set-examples/
│   └── image-set-config.yaml         # example ImageSetConfiguration
└── okd-mirror.md                     # OKD-specific mirroring notes
```

## Key architecture components

The project uses [go.podman.io/image/v5](https://github.com/containers/image) for low-level container image transport and copy operations.

For each image type, `oc-mirror` defines a `CollectorInterface` responsible for discovering all related images to mirror:

| Collector | Location | Purpose |
|-----------|----------|---------|
| Release | `internal/pkg/release` | OpenShift release payloads |
| Operator | `internal/pkg/operator`| Red Hat operator catalogs |
| Helm | `internal/pkg/helm`| Helm charts |
| Additional | `internal/pkg/additional` | Generic container images |

Each collector returns `[]v2alpha1.CopyImageSchema` — a list of source/destination image pairs. The image copying itself happens concurrently in batches via `internal/pkg/batch`.

The `ImageSetConfiguration` types are defined in `internal/pkg/api/v2alpha1/`. See `docs/image-set-examples/image-set-config.yaml` for an example.

## Common development commands

### Building

```bash
make build       # build v1 + v2 binary into ./bin/
make clean       # remove build artifacts
make cross-build # cross-compile for amd64, arm64, ppc64le, s390x
```

**Important**: always use `make build`, not `go build` directly — the Makefile sets required build tags (`json1`, btrfs/libdm exclusions, etc.) and embeds the v1 binary.

Individual cross-build targets are also available: `cross-build-linux-amd64`, `cross-build-linux-arm64`, `cross-build-linux-ppc64le`, `cross-build-linux-s390x`.

### Testing

```bash
make test-unit        # unit tests (./internal/pkg/... -short)
make test-integration # integration tests (runs specific Integration* test funcs)
make cover            # generate HTML coverage report from unit test results
```

Unit tests write coverage to `tests/results/cover.out`. Integration tests write to `tests/results-integration/`.

To run a single test or package directly, you **must** pass the build tags:

```bash
go test -tags "json1 exclude_graphdriver_devicemapper exclude_graphdriver_btrfs containers_image_openpgp" \
  -short -race -count=1 ./internal/pkg/image/...

go test -tags "json1 exclude_graphdriver_devicemapper exclude_graphdriver_btrfs containers_image_openpgp" \
  -short -race -count=1 -run TestSpecificName ./internal/pkg/release/...
```

### Validation and verification

```bash
make verify  # run golangci-lint
make vet     # run go vet
make format  # check gofmt compliance
make tidy    # run go mod tidy
make sanity  # runs tidy + format + vet, then fails if working tree is dirty
make all     # clean + tidy + build + test (full pipeline)
```

Run `make sanity` before committing — it will fail if there are uncommitted formatting or module changes.

## Testing patterns

- **Unit vs integration**: `make test-unit` runs with `-short`. Integration tests check `testing.Short()` and skip themselves, so they only run via `make test-integration`.
- **Mocking**: tests use `github.com/stretchr/testify/mock`. Mock structs are defined alongside the tests that use them (see `internal/pkg/cli/executor_test.go` for examples).
- **Test fixtures**: stored in `tests/` and referenced via the constant `consts.TestFolder` (`"../../../tests/"`). Fixtures include fake OCI images, caches, configs, and archive data.
- **Assertions**: the project uses `github.com/stretchr/testify/assert` and `require`.

## Contributing

1. Write understandable code. Always prefer clarity over other things.
1. Write comments and documentation in English.
1. Write unit tests for your code.
1. When instructed to fix tests, do not remove or modify existing tests.
1. Write documentation for your code.
1. Run `make sanity` before committing files.
