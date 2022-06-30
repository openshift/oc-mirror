Design: oc-mirror Metadata Management
===
- [Design: oc-mirror Metadata Management](#design-oc-mirror-metadata-management)
  - [Overview](#overview)
  - [Where is metadata stored?](#where-is-metadata-stored)
  - [How can you interact with metadata through the `oc-mirror` CLI?](#how-can-you-interact-with-metadata-through-the-oc-mirror-cli)
    - [Describe](#describe)
    - [Ignore History](#ignore-history)
    - [Skip Metadata Check](#skip-metadata-check)
    - [Why do we use a sequence number?](#why-do-we-use-a-sequence-number)

## Overview
oc-mirror depends on a metadata file to perform differentials operations such as upgrade graph calculations, layer deduplication, image download tracking, and pruning.

## Where is metadata stored?

When interacting with `oc-mirror` in a connected environment, the imageset configuration will define where the metadata is stored, through the
`storageConfig` key. In the target mirror registry, the metadata is also stored for sequence checking (See [overview](overview.md) for information on sequences). The exact image location will always be `<user-defined registry>/<user-defined namespace>/oc-mirror:<metadata-uuid>`. The uuid can be obtained using the `oc-mirror` describe command explained below.

## How can you interact with metadata through the `oc-mirror` CLI?

1. `oc-mirror` describe
2. `oc-mirror` with `--ignore-history` flags
3. `oc-mirror` with `--skip-metadata-check`

### Describe

`oc-mirror describe` allows the metadata to be viewed in an archived imageset. It takes one argument which is the path to directory with the archive or the path to an archive file. This can be used in the event that an image is deleted from the `oc-mirror` managed registry.

### Ignore History

By default, `oc-mirror` will not re-download images or blob detected in the past runs of the tools. If an image does not needs to be re-downloaded, the `--ignore-history` flag can be used to ignore the metadata in the mirror planning phase.

### Skip Metadata Check

In disk to mirror workflows, the metadata sequence is checked against previously mirrored imagesets to ensure no erros occur when reconstituting images before publishing. In the event that a sequence archived is lost, the `skip-metadata-check` flag can be used. To get the workspace back into a healthy state, perform the following tasks:

- Create a new imageset with `--ignore-history` to ensure all images and blob are packed into the archive
- Publish the imageset with `--skip-metadata-check` to allow the imageset to overwrite the sequence number

### Why do we use a sequence number?

`oc-mirror` performs deduplication at the image layer level when mirroring images. Instead of packing layers that have already been mirrored, the layers are retrieving from the managed registry during the publishing when needed to reconstitute an image. Ensuring the imagesets are published in order ensures that the expected layers were published to the registry.
