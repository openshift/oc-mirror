# oc-mirror v2 Architecture

An overview of the oc-mirror v2 architecture for contributors and developers.

## What oc-mirror Does

oc-mirror mirrors container images from source registries to a destination, supporting disconnected OpenShift cluster installs. It reads an `ImageSetConfiguration` YAML and operates in three workflow modes:

| Mode | Usage | Description |
|------|-------|-------------|
| **Mirror to Disk (M2D)** | `oc-mirror ... file://<path>` | Pull images into local cache, produce a tar archive |
| **Disk to Mirror (D2M)** | `oc-mirror --from file://<path> docker://<registry>` | Extract archive, push images to destination registry |
| **Mirror to Mirror (M2M)** | `oc-mirror --workspace file://<path> docker://<registry>` | Direct registry-to-registry copy |

## High-Level Flow

```
ImageSetConfiguration (YAML)
         |
    Read config
         |
    Initialize                  <- determine workflow mode, set up modules
         |
    Start execution             <- start local cache registry, dispatch to workflow
         |
         +--- Mirror to Disk / Disk to Mirror / Mirror to Mirror
         |
    Collect all images
    |    |    |    |
 Release Op  Add  Helm          <- each produces []CopyImageSchema
    |    |    |    |
    +----+----+----+
         |
    Exclude blocked + deduplicate
         |
    Rebuild catalogs            <- M2D and M2M only
         |
    Batch worker                <- concurrent image copy
         |
    Build archive (M2D)  /  Cluster resources generation (D2M, M2M)
```

## Key Directories

- `cmd/oc-mirror/` -- binary entrypoint (v1/v2 dispatch)
- `internal/pkg/cli/` -- executor, command setup, workflow orchestration
- `internal/pkg/api/v2alpha1/` -- all API types (ImageSetConfiguration, CopyImageSchema, etc.)
- `internal/pkg/config/` -- configuration loading, validation, and defaults
- `internal/pkg/batch/` -- concurrent image copy/delete workers
- `internal/pkg/archive/` -- tar archive creation and extraction
- `internal/pkg/clusterresources/` -- generates IDMS/ITMS, CatalogSource, UpdateService YAMLs
- `docs/` -- documentation and ImageSetConfiguration examples

## Executor Lifecycle

The executor (`internal/pkg/cli`) orchestrates the entire workflow:

1. **Validate** -- checks CLI flags and destination protocol (`file://` or `docker://`)
2. **Complete** -- reads the `ImageSetConfiguration`, determines the workflow mode from the destination/flags, and initializes all module dependencies (collectors, mirror, batch worker, archive, cluster resources, image builders)
3. **Run** -- starts the local cache registry, dispatches to the appropriate workflow method, stops the registry on completion

## Collector System

Each content type has a collector that discovers images and produces `[]CopyImageSchema` entries (source/destination pairs ready for copying).

| Collector | Package | Discovers |
|-----------|---------|-----------|
| Release | `internal/pkg/release` | OpenShift release payloads via Cincinnati API |
| Operator | `internal/pkg/operator` | Operator catalogs, bundles, and related images via FBC parsing |
| Additional | `internal/pkg/additional` | Generic container images listed in the config |
| Helm | `internal/pkg/helm` | Images referenced in Helm chart templates |

`CollectAll()` runs all collectors, applies `BlockedImages` exclusions, removes duplicates, and sorts results so catalogs are processed first.

## Key Data Structures

**CopyImageSchema** (`internal/pkg/api/v2alpha1`) -- the central unit of work representing one image to copy:
- `Source` / `Destination` -- full image references with transport prefix
- `Origin` -- original image reference for tracking
- `Type` -- an `ImageType` enum classifying the image (e.g., `TypeOCPRelease`, `TypeOperatorCatalog`, `TypeGeneric`)

**CollectorSchema** -- aggregated output from all collectors, carrying the total image counts, the full `[]CopyImageSchema` list, and operator-specific mappings.

**ImageSetConfiguration** (`internal/pkg/api/v2alpha1`) -- the user-facing YAML config defining what to mirror: `Platform` (OCP releases), `Operators` (catalogs), `AdditionalImages`, `Helm` (charts), and `BlockedImages` (exclusion patterns).

## Batch Worker

The batch worker (`internal/pkg/batch`) copies images concurrently using goroutines controlled by a semaphore (default 4, configurable via `--parallel-images`). Operator bundles are skipped if any of their related images failed to copy. Errors are collected and written to a report file.

## Archive System

- **M2D**: creates a tar archive containing cached manifests, blobs (incremental via history), the working directory, and the `ImageSetConfiguration`.
- **D2M**: extracts the tar archive back into the cache and working directory.
- **History** (`internal/pkg/history`): tracks which blobs have been archived previously, enabling incremental archives. Controlled by the `--since` flag.

## Local Cache Registry

oc-mirror runs an embedded Docker Distribution registry on localhost (default port 55000) as a local cache. All three workflows use it:
- **M2D**: images are copied source -> local cache, then archived
- **D2M**: archive is extracted to cache, images copied cache -> destination
- **M2M**: operator catalogs are cached locally; other images go direct

## Cluster Resources Generation

During D2M and M2M workflows, oc-mirror generates Kubernetes YAML resources in `working-dir/cluster-resources/`:

| Generator | Output |
|-----------|--------|
| `IDMS_ITMSGenerator` | `ImageDigestMirrorSet` / `ImageTagMirrorSet` for cluster image policy |
| `CatalogSourceGenerator` | `CatalogSource` for OLM |
| `ClusterCatalogGenerator` | `ClusterCatalog` for OLM v1 |
| `UpdateServiceGenerator` | `UpdateService` for Cincinnati (when `graph: true`) |
| `GenerateSignatureConfigMap` | `ConfigMap` with release image signatures |

## Delete Workflow

A two-phase process managed by the delete sub-command (`internal/pkg/cli`):

1. **Generate** (`--generate`): reads a `DeleteImageSetConfiguration`, runs collectors to discover images, writes a `delete-images-<id>.yaml` manifest
2. **Execute** (`--delete-yaml-file`): reads the manifest and deletes images from the target registry

## Catalog Rebuilding

When operator catalogs are filtered (not mirroring the full catalog), oc-mirror rebuilds the catalog image to contain only the filtered FBC (File-Based Catalog) content. This is handled by the catalog builder (`internal/pkg/imagebuilder`) during M2D and M2M workflows, before the batch copy step.
