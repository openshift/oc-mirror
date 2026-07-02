# Mirroring Workflows

## Overview

oc-mirror supports three core mirroring workflows to handle different connectivity scenarios between source registries and destination environments. Each workflow is selected based on the command-line arguments and destination prefix.

| Workflow | Use case | Command pattern |
|----------|----------|----------------|
| Mirror-to-Disk (m2d) | Air-gapped: download images to a local archive | `oc-mirror -c <config> file://<path>` |
| Disk-to-Mirror (d2m) | Air-gapped: upload archive contents to a registry | `oc-mirror -c <config> --from file://<path> docker://<registry>` |
| Mirror-to-Mirror (m2m) | Partially disconnected: copy directly between registries | `oc-mirror -c <config> --workspace file://<path> docker://<registry>` |

## Prerequisites

### Authentication

oc-mirror retrieves registry credentials from the default podman credentials location (`$XDG_RUNTIME_DIR/containers/auth.json`). Docker credentials at `~/.docker/config.json` are also supported as a secondary location.

Ensure your [Red Hat OpenShift Pull Secret](https://console.redhat.com/openshift/install/pull-secret) and any other needed registry credentials are present in the credentials file.

### Certificate trust

oc-mirror references the host system for certificate trust. Add all certificates in the trust chain to the [system-wide trust store](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/7/html/security_guide/sec-shared-system-certificates).

## Mirror-to-Disk (m2d)

Pulls container images from the sources defined in the ImageSetConfiguration and packs them into a tar archive on disk.

```bash
oc-mirror -c ./isc.yaml file:///home/user/oc-mirror/output
```

**What it produces:**
- A tar archive (`mirror_*.tar`) under the specified directory containing all collected images
- A `working-dir/` directory at the destination location with internal artifacts and logs

**When to use:** When you need to transfer images across an air gap. The archive can be physically transported (USB drive, SFTP, etc.) to the disconnected environment.

**Key flags:**
- `--since yyyy-MM-dd` — Only include content mirrored after the specified date
- `--force` — Force copy even if nothing needs updating
- `--strict-archive` — Fail if any single file exceeds the `archiveSize` limit set in the ImageSetConfiguration

See [Archive Management](archive-management.md) for details on archive segmentation and incremental mirroring.

## Disk-to-Mirror (d2m)

Copies images from a previously created tar archive to a destination container registry.

```bash
oc-mirror -c ./isc.yaml --from file:///home/user/oc-mirror/output docker://registry.example.com
```

**What it produces:**
- Images pushed to the destination registry
- Cluster resource manifests (IDMS, ITMS, CatalogSource, etc.) in `working-dir/cluster-resources/`

**When to use:** After transferring an m2d archive into a disconnected environment and making it available on the local filesystem.

**Key flags:**
- `--from file://<path>` — Path to the directory containing the archive (required for d2m)
- `-c` / `--config` — Still required; allows mirroring a subset of the archive contents

See [Cluster Resources](cluster-resources.md) for details on the generated manifests.

## Mirror-to-Mirror (m2m)

Copies images directly from source registries to the destination registry without creating an intermediate archive. Requires network access to both source and destination.

```bash
oc-mirror -c ./isc.yaml --workspace file:///home/user/oc-mirror/workspace docker://registry.example.com
```

**What it produces:**
- Images copied directly to the destination registry
- Cluster resource manifests in `<workspace>/working-dir/cluster-resources/`

**When to use:** When the machine running oc-mirror has network access to both the source registries (e.g., `registry.redhat.io`, `quay.io`) and the destination mirror registry.

**Key flags:**
- `--workspace file://<path>` — Local directory for the working-dir and internal artifacts (required for m2m)

## Common flags

| Flag | Description |
|------|-------------|
| `-c`, `--config` | Path to ImageSetConfiguration file (required) |
| `--dry-run` | Preview what would be mirrored without copying. See [Dry Run](dry-run.md) |
| `--parallel-images` | Number of images mirrored in parallel (default 10, max 10) |
| `--parallel-layers` | Number of image layers mirrored in parallel (default 10, max 10) |
| `--image-timeout` | Timeout for mirroring a single image (default 10m) |
| `--max-nested-paths` | Limit nested paths for registries that restrict path depth |
| `--log-level` | Log level: info, debug, trace, error (default info) |
| `--secure-policy` | Enable signature verification. See [Signature Verification](signature-verification.md) |
| `--cache-dir` | Override the default cache directory (`~/.oc-mirror`) |

## Incremental mirroring

All workflows support incremental mirroring. On subsequent runs with the same workspace/destination, oc-mirror tracks previously mirrored content via a history file and only processes new images. This avoids re-downloading or re-copying content that was already mirrored.

Use the `--since` flag to include only content mirrored after a specific date:

```bash
oc-mirror -c ./isc.yaml --since 2024-06-01 file:///home/user/oc-mirror/output
```

See [Archive Management](archive-management.md) for more details on incremental behavior.

## Working directory structure

oc-mirror creates a `working-dir/` directory at the workspace or destination location. This directory contains:

```
working-dir/
  cluster-resources/     # Generated IDMS, ITMS, CatalogSource, etc.
  logs/                  # Detailed operation logs
  dry-run/               # Dry-run output (when --dry-run is used)
  delete/                # Delete operation artifacts
```

## Workflow selection summary

The workflow is determined automatically based on the command-line arguments:

| Destination prefix | `--from` flag | `--workspace` flag | Workflow |
|-------------------|---------------|-------------------|----------|
| `file://` | not set | not set | Mirror-to-Disk |
| `docker://` | set (`file://`) | not set | Disk-to-Mirror |
| `docker://` | not set | set (`file://`) | Mirror-to-Mirror |

Invalid combinations (e.g., `file://` with `--from`, or `docker://` with both `--from` and `--workspace`) will produce an error.

## Related documentation

- [Enclave Support](enclave_support.md) — Multi-stage disconnected workflows with intermediate registries
- [Cluster Resources](cluster-resources.md) — What manifests are generated and how to apply them
- [Filtering](filtering.md) — How to control which content is mirrored
- [Archive Management](archive-management.md) — Archive segmentation and incremental mirroring
- [Dry Run](dry-run.md) — Preview mirroring operations
- [Delete Functionality](delete-functionality.md) — Removing previously mirrored images
