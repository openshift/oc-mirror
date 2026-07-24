# Dry Run

## Overview

The `--dry-run` flag allows you to preview a mirroring operation without actually copying any images. It produces a list of all images that would be mirrored and, for mirror-to-disk workflows, identifies which images are missing from the local cache.

## Usage

Add `--dry-run` to any mirroring command:

```bash
# Dry run for mirror-to-disk
oc-mirror -c ./isc.yaml --dry-run file:///home/user/oc-mirror/output

# Dry run for disk-to-mirror
oc-mirror -c ./isc.yaml --dry-run --from file:///home/user/oc-mirror/output docker://registry.example.com

# Dry run for mirror-to-mirror
oc-mirror -c ./isc.yaml --dry-run --workspace file:///home/user/oc-mirror/workspace docker://registry.example.com
```

## Output files

Dry-run output is written to `working-dir/dry-run/` under the workspace or destination directory.

### mapping.txt

Lists every image that would be mirrored, in `source=destination` format:

```
quay.io/openshift-release-dev/ocp-release@sha256:abc123...=registry.example.com/openshift-release-dev/ocp-release@sha256:abc123...
registry.redhat.io/redhat/redhat-operator-index:v4.18=registry.example.com/redhat/redhat-operator-index:v4.18
```

This file is always generated for all workflow types.

### missing.txt

For mirror-to-disk workflows only, lists images that are not yet available in the local cache. These images would need to be downloaded from the source registry during an actual mirroring run:

```
quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:def456...=docker://localhost:55000/openshift-release-dev/ocp-v4.0-art-dev@sha256:def456...
```

If all required images are already in the cache, `missing.txt` is not created and a message confirms that mirroring from disk to a disconnected registry can proceed.

## Cluster resources in dry-run mode

For mirror-to-mirror and disk-to-mirror dry runs, [cluster resources](cluster-resources.md) (IDMS, ITMS, CatalogSource, etc.) are also generated. This lets you preview the Kubernetes manifests that would be created.

Cluster resources are **not** generated for mirror-to-disk dry runs, since the target registry is not known at that stage.

If cluster resource generation encounters an error during dry run, it is logged as a warning rather than failing the operation.

## Related documentation

- [Mirroring Workflows](mirroring-workflows.md) — The three core mirroring modes
- [Cluster Resources](cluster-resources.md) — What manifests are generated
- [Filtering](filtering.md) — Controlling which content is included
