# oc-mirror Metadata Management
===
- [# oc-mirror Metadata Management](#-oc-mirror-metadata-management)
  - [Overview](#overview)
  - [Where is metadata stored?](#where-is-metadata-stored)
  - [How can you interact with metadata through the `oc-mirror` CLI?](#how-can-you-interact-with-metadata-through-the-oc-mirror-cli)
  
## Overview
oc-mirror depends on a metadata file to perform differentials operations such as upgrade graph calculations, layer deduplication, image download tracking, and pruning.

## Where is metadata stored?

When interacting with `oc-mirror` in a connected environment, the imageset configuration will define where the metadata is stored, through the
`storageConfig` key. In the target mirror registry, the metadata is also stored for sequence checking (See [overview](overview.md) for information on sequences). The exact image location will always be `<user-defined registry>/<user-defined namespace>/oc-mirror:<metadata-uuid>`. The uuid can obtained using the `oc-mirror` describe command explained below.

## How can you interact with metadata through the `oc-mirror` CLI?

1. `oc-mirror` describe
2. `oc-mirror` with `--ignore-history flags
3. `oc-mirror` with `--skip-metadata-check`


