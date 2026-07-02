# Archive Management and Incremental Mirroring

## Overview

In disconnected (air-gapped) workflows, oc-mirror creates tar archives containing mirrored images and metadata. These archives are transported across network boundaries and then published to a destination registry. oc-mirror tracks previously mirrored content to enable incremental updates, so only new images are included in subsequent archives.

This document covers how archives work, how to configure their size, how incremental mirroring works, and how to recover from common issues.

## Archive structure

When running mirror-to-disk, oc-mirror creates tar archives under the specified `file://` destination. The archive naming convention is `mirror_*.tar`, and each archive contains:

- **`docker/v2/repositories/`** — Image manifests for all mirrored images
- **`docker/v2/blobs/sha256/`** — Image layer blobs (only new/changed blobs in incremental runs)
- **`working-dir/`** — Internal metadata, cluster resources, and logs
- **Image set configuration** — A timestamped copy of the ISC used for the run

## Archive segmentation

Large mirror sets can produce archives that exceed the capacity of the transport medium. The `archiveSize` field in the ImageSetConfiguration specifies the maximum size (in GB) for each archive segment:

```yaml
apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
archiveSize: 4
mirror:
  platform:
    channels:
      - name: stable-4.18
```

When the archive exceeds the configured size, oc-mirror splits it into multiple numbered segments. If `archiveSize` is not set, the default maximum is 500 GB.

### Strict vs. permissive archiving

By default, oc-mirror uses **permissive** archiving: if a single file (e.g., a large image layer) exceeds the `archiveSize` limit, it is included anyway and a warning is logged. The archive segment may be larger than the configured maximum.

Use the `--strict-archive` flag for **strict** archiving: the operation fails with an error if any individual file exceeds the `archiveSize` limit. This is useful when transport media have hard size constraints.

```bash
oc-mirror -c ./isc.yaml --strict-archive file:///home/user/output
```

## Local cache

oc-mirror maintains a local cache for image data. The default cache location is `~/.oc-mirror`, and it can be configured with:

- The `--cache-dir` flag: `oc-mirror -c ./isc.yaml --cache-dir /custom/cache file:///home/user/output`
- The `OC_MIRROR_CACHE` environment variable

These two options are mutually exclusive.

The cache stores downloaded image layers and manifests between runs. In mirror-to-disk workflows, the archive contains only the blobs that are new since the last run, while the cache retains the complete set of previously downloaded data.

## Incremental mirroring

oc-mirror tracks what has been mirrored previously and only includes new content in subsequent runs. This is handled through a history mechanism.

### How it works

1. During each mirror-to-disk run, oc-mirror reads the history file from the `working-dir/` to determine which blobs were included in previous archives.
2. Only blobs not found in the history are added to the new archive.
3. After the archive is built, the history file is updated with the newly added blobs.

This means the first run produces a full archive, and subsequent runs produce smaller differential archives containing only new content.

### Using `--since`

The `--since` flag allows you to include all content mirrored after a specific date, overriding the default incremental behavior:

```bash
oc-mirror -c ./isc.yaml --since 2024-06-01 file:///home/user/output
```

The date format is `yyyy-MM-dd`. When `--since` is specified, oc-mirror considers only history entries after the given date when calculating the differential.

### Using `--force`

The `--force` flag forces oc-mirror to re-copy all content regardless of what was previously mirrored:

```bash
oc-mirror -c ./isc.yaml --force file:///home/user/output
```

## Extracting archives (disk-to-mirror)

To publish an archive to a registry, use the `--from` flag to point to the directory containing the archive:

```bash
oc-mirror -c ./isc.yaml --from file:///home/user/output docker://registry.example.com
```

oc-mirror extracts the archive, loads the image data, and pushes it to the destination registry.

The `-c` / `--config` flag is still required for disk-to-mirror. The configuration allows mirroring only a subset of the archive contents, which is useful when a single archive is intended for multiple destination environments (e.g., different enclaves).

## Archive cleanup

Previous archives in the destination directory are automatically removed before a new mirror-to-disk run. The `mirror_*.tar` files from the prior run are deleted to avoid accumulating outdated archives.

**Important:** If you need to preserve previous archives for recovery purposes, copy or move them to a separate location before running oc-mirror again.

## Recovery scenarios

### Lost archive

If an archive is lost during transport, re-run oc-mirror with the `--force` flag to regenerate a full archive:

```bash
oc-mirror -c ./isc.yaml --force file:///home/user/output
```

### Corrupted cache

If the local cache becomes corrupted, delete the cache directory and re-run oc-mirror. A fresh cache will be built from scratch:

```bash
rm -rf ~/.oc-mirror
oc-mirror -c ./isc.yaml file:///home/user/output
```

### Restoring cache from archives

If the cache is lost but you have the tar archives, extract the `docker/` directory from the archives and copy its contents to the cache directory:

```bash
tar xf mirror_*.tar -C ~/.oc-mirror/.cache docker/
```

## Related documentation

- [Mirroring Workflows](mirroring-workflows.md) — The three core mirroring modes
- [Enclave Support](enclave_support.md) — Multi-stage disconnected workflows
- [Filtering](filtering.md) — Controlling which content is mirrored
- [Dry Run](dry-run.md) — Preview operations including cache status
