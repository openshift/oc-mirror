# oc-mirror

`oc-mirror` is an OpenShift CLI tool for mirroring container images to disconnected and partially disconnected environments.

It reads a declarative `ImageSetConfiguration` to discover OpenShift release payloads, operator catalogs, Helm charts, and additional container images, then copies them to a target mirror registry or local archive.

- [oc-mirror](#oc-mirror)
  - [Overview](#overview)
    - [Destination prefixes](#destination-prefixes)
    - [Authentication](#authentication)
  - [Quick Start](#quick-start)
    - [Prerequisites](#prerequisites)
    - [Build](#build)
  - [Usage](#usage)
    - [Image Set Configuration](#image-set-configuration)
    - [Mirror to Disk](#mirror-to-disk)
    - [Disk to Mirror](#disk-to-mirror)
    - [Mirror to Mirror](#mirror-to-mirror)
    - [Delete Subcommand](#delete-subcommand)
      - [Delete Image Set Configuration](#delete-image-set-configuration)
      - [Delete Phase 1](#delete-phase-1)
      - [Delete Phase 2](#delete-phase-2)
    - [List Subcommand](#list-subcommand)
      - [List releases](#list-releases)
      - [List operators](#list-operators)
  - [Flags Reference](#flags-reference)
    - [Global flags](#global-flags)
    - [Mirror command flags](#mirror-command-flags)
    - [Delete subcommand flags](#delete-subcommand-flags)
    - [List subcommand flags](#list-subcommand-flags)
      - [`list releases`](#list-releases-1)
      - [`list operators`](#list-operators-1)
  - [Features](#features)
    - [Cluster Resources](#cluster-resources)
    - [Catalog Pinning](#catalog-pinning)
    - [Additional Feature Documentation](#additional-feature-documentation)
  - [Repository Guidance](#repository-guidance)
  - [Security](#security)
  - [License](#license)

## Overview

Three mirroring workflows are available:

| Workflow | Short name | Description |
|----------|-----------|-------------|
| Mirror to Disk | `m2d` | Pull images from source registries and pack them into a tar archive on disk. |
| Disk to Mirror | `d2m` | Unpack a tar archive and push images to a target container registry. |
| Mirror to Mirror | `m2m` | Copy images directly from source registries to a target container registry. |

**Mirror to Disk** and **Disk to Mirror** are used together for fully air-gapped environments. `m2d` runs on a machine with internet access to create the archive, then `d2m` runs in the disconnected environment to push images to the local registry. For enclave scenarios, `m2d` can also pull from an internal registry using a `registries.conf` file.

**Mirror to Mirror** is used for partially disconnected environments where the machine running the tool has access to both the internet and the target registry.

### Destination prefixes

When specifying the destination on the command line:

- `file://<path>` — local directory for the tar archive (used with `m2d`)
- `docker://<registry>` — container registry (used with `d2m` and `m2m`)

### Authentication

The default podman credentials location (`$XDG_RUNTIME_DIR/containers/auth.json`) is used for authenticating to container registries. The Docker location (`~/.docker/config.json`) is also supported as a fallback.

> **Note:** oc-mirror v1 was deprecated in OCP 4.18 and will be removed in a future release. The v1 README is available [here](v1/README_v1.md).

## Quick Start

### Prerequisites

- Git
- Go (see `go.mod` for minimum version)

### Build

```bash
git clone https://github.com/openshift/oc-mirror.git
cd oc-mirror
make build
```

The binary is created at `bin/oc-mirror`. You can use oc-mirror as an `oc` plugin by placing the binary in a directory on your `$PATH`.

## Usage

### Image Set Configuration

The Image Set Configuration (`ImageSetConfiguration`) is the declarative input that tells oc-mirror what to mirror. Pass it with the `-c` or `--config` flag.

```yaml
kind: ImageSetConfiguration
apiVersion: mirror.openshift.io/v2alpha1
mirror:
  platform:
    channels:
    - name: stable-4.18
      minVersion: 4.18.1
      maxVersion: 4.18.1
    graph: true
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.18
      packages:
       - name: aws-load-balancer-operator
       - name: 3scale-operator
       - name: node-observability-operator
  additionalImages:
   - name: registry.redhat.io/ubi9/ubi:latest
  helm:
    repositories:
      - name: cosigned
        url: https://sigstore.github.io/helm-charts
        charts:
          - name: cosigned
            version: 0.1.23
    local:
     - name: my-local-chart
       path: /path/to/local/chart
```

More examples can be found in the [image set examples](docs/image-set-examples/) directory.

### Mirror to Disk

```bash
oc-mirror -c ./isc.yaml file:///home/<user>/oc-mirror/mirror1 --v2
```

### Disk to Mirror

```bash
oc-mirror -c ./isc.yaml --from file:///home/<user>/oc-mirror/mirror1 docker://localhost:6000 --v2
```

### Mirror to Mirror

```bash
oc-mirror -c ./isc.yaml --workspace file:///home/<user>/oc-mirror/mirror1 docker://localhost:6000 --v2
```

### Delete Subcommand

The `delete` subcommand removes images from a remote registry. It operates in two phases:

1. **Phase 1 (`--generate`):** Discovers all images to delete based on a `DeleteImageSetConfiguration` and writes them to a YAML file.
2. **Phase 2:** Reads the generated YAML file and deletes the image manifests from the target registry. The registry's garbage collector is responsible for cleaning up unreferenced blobs.

For full details, see [Delete Functionality](docs/features/delete-functionality.md).

#### Delete Image Set Configuration

```yaml
kind: DeleteImageSetConfiguration
apiVersion: mirror.openshift.io/v2alpha1
delete:
  platform:
    channels:
    - name: stable-4.18
      minVersion: 4.18.1
      maxVersion: 4.18.1
    graph: true
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.18
      packages:
       - name: aws-load-balancer-operator
       - name: 3scale-operator
       - name: node-observability-operator
  additionalImages:
   - name: registry.redhat.io/ubi9/ubi:latest
```

#### Delete Phase 1

```bash
oc-mirror delete -c ./delete-isc.yaml --generate --workspace file:///home/<user>/oc-mirror/delete1 --delete-id delete1-test docker://localhost:6000 --v2
```

#### Delete Phase 2

```bash
oc-mirror delete --delete-yaml-file /home/<user>/oc-mirror/delete1/working-dir/delete/delete-images-delete1-test.yaml docker://localhost:6000 --v2
```

### List Subcommand

The `list` subcommand queries available platform releases and operator content without mirroring anything.

#### List releases

```bash
# List all available OpenShift release versions
oc-mirror --v2 list releases

# List releases for a specific version
oc-mirror --v2 list releases --version=4.18

# List releases in a specific channel
oc-mirror --v2 list releases --channel=stable-4.18

# List releases filtered by architecture (valid: amd64, arm64, ppc64le, s390x, multi)
oc-mirror --v2 list releases --channel=fast-4.18 --version=4.18 --filter-by-archs amd64,arm64

# List all channels for a specific version
oc-mirror --v2 list releases --channels --version=4.18
```

#### List operators

```bash
# List available catalogs for an OpenShift version
oc-mirror --v2 list operators --catalogs --version=4.18

# List all packages in a catalog
oc-mirror --v2 list operators --catalog=registry.redhat.io/redhat/redhat-operator-index:v4.18

# List all channels for a package
oc-mirror --v2 list operators --catalog=registry.redhat.io/redhat/redhat-operator-index:v4.18 --package=aws-load-balancer-operator

# List versions in a specific channel
oc-mirror --v2 list operators --catalog=registry.redhat.io/redhat/redhat-operator-index:v4.18 --package=aws-load-balancer-operator --channel=stable-v1
```

## Flags Reference

### Global flags

```
      --authfile string                path of the authentication file. Default is ${XDG_RUNTIME_DIR}/containers/auth.json
      --cache-dir string               oc-mirror cache directory location. Default is $HOME
  -c, --config string                  Path to imageset configuration file
      --dest-tls-verify                require HTTPS and verify certificates when talking to the container registry or daemon (default true)
      --log-level string               Log level one of (info, debug, trace, error) (default "info")
      --parallel-images uint           Number of images mirrored in parallel (default 4)
      --parallel-layers uint           Number of image layers mirrored in parallel (default 5)
      --policy string                  Path to a trust policy file
  -p, --port uint16                    HTTP port used by oc-mirror's local storage instance (default 55000)
      --registries.d DIR               use registry configuration files in DIR (e.g. for container signature storage)
      --retry-delay duration           delay between retries (default 0 uses exponential backoff, set value uses constant delay)
      --retry-times int                the number of times to possibly retry (default 5)
      --src-tls-verify                 require HTTPS and verify certificates when talking to the container registry or daemon (default true)
      --workspace string               oc-mirror workspace where resources and internal artifacts are generated
  -v, --version                        version for oc-mirror
  -h, --help                           help for oc-mirror
```

### Mirror command flags

```
      --dry-run                        Print actions without mirroring images
      --dry-run-manifest-lists         Like --dry-run, but also includes manifest list sub-digests in mapping.txt (implies --dry-run)
      --from string                    Local storage directory for disk to mirror workflow
      --ignore-release-signature       Ignore release signature
      --image-timeout duration         Timeout for mirroring an image (default 10m0s)
      --max-nested-paths int           Number of nested paths, for destination registries that limit nested paths
      --remove-signatures              Do not copy image signatures
      --rootless-storage-path string   Override the default container rootless storage path
      --secure-policy                  Enable signature verification (secure policy for signature verification)
      --since string                   Include all new content since specified date (format yyyy-MM-dd)
      --strict-archive                 Generate archives strictly less than archiveSize (set in the ImageSetConfiguration)
```

### Delete subcommand flags

```
      --delete-id string          Identifier to differentiate between versions of delete output files
      --delete-signatures         Delete container image signatures (for multi-arch, deletes only the manifest list signature)
      --delete-v1-images          Target images previously mirrored with oc-mirror v1 (used with --generate)
      --delete-yaml-file string   Path to the generated or updated YAML file for deleting contents
      --force-cache-delete        Force delete local cache manifests and blobs
      --generate                  Generate the delete YAML listing manifests and blobs to delete
```

### List subcommand flags

#### `list releases`

```
      --channel string            List information for a specific channel (defaults to stable)
      --channels                  List all channel information (requires --version)
      --filter-by-archs strings   Architecture filter for release images (default [amd64])
      --version string            OpenShift release version
```

#### `list operators`

```
      --catalog string   List information for a specified catalog
      --catalogs         List available catalogs for an OpenShift release version (requires --version)
      --channel string   List information for a specified channel (requires --catalog and --package)
      --package string   List information for a specified package (requires --catalog)
      --version string   OpenShift release version
```

## Features

### Cluster Resources

After a successful `d2m` or `m2m` workflow, oc-mirror generates cluster resources that must be applied to the OpenShift cluster so it can pull images from the mirror registry:

- **ImageDigestMirrorSet (IDMS)** — maps image digests to the mirror registry
- **ImageTagMirrorSet (ITMS)** — maps image tags to the mirror registry
- **CatalogSource** — registers mirrored operator catalogs with OLM
- **ClusterCatalog** — registers mirrored catalogs for the cluster catalog framework
- **UpdateService** — configures OpenShift Update Service (OSUS) for disconnected updates

These resources are generated under `<workspace>/working-dir/cluster-resources`.

### Catalog Pinning

When running `m2d` or `m2m` workflows, oc-mirror automatically generates pinned configuration files where operator catalogs are referenced by SHA256 digest instead of tag. This ensures reproducible mirroring since digest references always resolve to the exact same image content.

Two files are generated in the working directory:
- `isc_pinned_{timestamp}.yaml` — pinned `ImageSetConfiguration` for reproducible re-mirrors
- `disc_pinned_{timestamp}.yaml` — corresponding `DeleteImageSetConfiguration` for precise cleanup

### Additional Feature Documentation

- [Enclave Support](docs/features/enclave_support.md) — multi-stage mirroring through intermediate disconnected networks
- [Signature Verification](docs/features/signature-verification.md) — verifying image signatures during mirroring
- [Delete Functionality](docs/features/delete-functionality.md) — removing mirrored images from a registry

## Repository Guidance

- [CONTRIBUTING.md](CONTRIBUTING.md) — contributor workflow, development setup, testing, and PR guidelines
- [ARCHITECTURE.md](ARCHITECTURE.md) — system design, component relationships, and data flow

## Security

If you find a security issue in this project, do **not** file it as a public GitHub issue. Instead, report it confidentially at https://access.redhat.com/security/team/contact.

## License

oc-mirror is licensed under the [Apache License, Version 2.0](http://www.apache.org/licenses/LICENSE-2.0).
