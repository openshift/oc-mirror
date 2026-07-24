# AI Agent Guide for oc-mirror

This file provides guidance to AI agents when working with code in this repository.

## Project Overview

This is the `oc-mirror` repository — an OpenShift command-line tool for mirroring container registry content for disconnected cluster installs.
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

## Architecture

### Data flow

The overall pipeline follows this flow:

```text
ImageSetConfiguration → Collectors → Batch Worker → Output (Archive or ClusterResources)
                                                   → Metadata persisted for incremental runs
```

1. The user provides an `ImageSetConfiguration` YAML describing what to mirror (releases, operators, helm charts, additional images).
2. **Collectors** read that configuration and discover all container images that need to be mirrored, returning normalized `CopyImageSchema` (source/destination pairs).
3. The **Batch Worker** copies images concurrently using goroutine semaphores.
4. Depending on the workflow, output is either a tar archive (m2d) or Kubernetes resources pointing the cluster at the mirror registry (m2m, d2m).
5. **Metadata** is persisted so subsequent runs can perform incremental mirroring.

### Mirror orchestration

The three workflows are orchestrated by executor types that wire together collectors, batch workers, and output generators:

- **MirrorToMirror**: discovers images, rebuilds operator catalogs, copies images directly to the target registry, and generates Kubernetes resources (IDMS/ITMS, CatalogSource, etc.)
- **MirrorToDisk**: discovers images, rebuilds operator catalogs, copies images to a local cache, and packages everything into a tar archive for transport
- **DiskToMirror**: extracts a previously created archive, discovers the images within it, copies them to the target registry, and generates Kubernetes resources

### Collectors

Each image type has a dedicated `CollectorInterface` implementation that discovers images to mirror:

| Collector | Purpose |
|-----------|---------|
| **Release** | Discovers OpenShift/OKD release payload images by querying the Cincinnati update graph for the requested channels and version ranges |
| **Operator** | Discovers operator catalog images, applies filtering (by package, channel, version range), and identifies all related bundle images |
| **Helm** | Discovers container images referenced by Helm charts |
| **Additional** | Passes through user-specified additional container images |

Release, Helm, and Additional collectors return `[]v2alpha1.CopyImageSchema` — a list of source→destination image pairs. The Operator collector returns `v2alpha1.CollectorSchema`, which wraps `CopyImageSchema` along with additional operator-specific metadata. All image lists are ultimately aggregated into a `CollectorSchema` by `CollectAll()` so the batch worker can process them uniformly.

### Batch Worker

The batch worker (`ChannelConcurrentBatch`) handles concurrent image copy/delete operations. It uses goroutine semaphores and channels to limit parallelism. It tracks errors per image type and handles operator bundle dependencies (skipping bundles whose related images failed).

### ImageSetConfiguration

The `ImageSetConfiguration` type defines what content to mirror. Its spec includes sections for platform releases (channels, architectures, update graph), operators (packages, channels, version ranges), additional images, Helm charts, blocked images, and archive size limits for segmented tarballs.

### Archive

The archive package handles packaging mirrored images into tar archives (used by MirrorToDisk) and extracting archives back to the filesystem (used by DiskToMirror). This enables the air-gapped workflow where images are transported physically.

### Cluster Resources

After mirroring, oc-mirror generates Kubernetes resources that configure the target cluster to use the mirror registry:

- **IDMS** (ImageDigestMirrorSet) and **ITMS** (ImageTagMirrorSet) — redirect image pulls to the mirror
- **CatalogSource** / **ClusterCatalog** — point the cluster's OLM at mirrored operator catalogs
- **UpdateService** — configure Cincinnati graph for mirrored release channels

### History

The `History` interface (`internal/pkg/history`) tracks which images were synced across runs via `Read()` and `Append()` methods. This enables incremental mirroring — subsequent runs only copy new or updated images.

### Image transport

The project uses [podman-container-tools/container-libs](https://github.com/podman-container-tools/container-libs/tree/main/image) for low-level container image transport and copy operations.

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

See [docs/testing/](docs/testing/) for the full testing strategy, conventions, and examples.

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
make all     # clean + tidy + build (full pipeline)
```

Run `make sanity` before committing — it will fail if there are uncommitted formatting or module changes.

## Contributing

1. Write understandable code. Always prefer clarity over other things.
1. Write comments and documentation in English.
1. Write unit tests for your code.
1. When instructed to fix tests, do not remove or modify existing tests.
1. Write documentation for your code.
1. Run `make sanity` before committing files.
