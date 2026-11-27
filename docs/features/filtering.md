# Content Filtering and Version Selection

## Overview

The ImageSetConfiguration controls which content oc-mirror collects and mirrors. Filtering applies across all four content types: platform releases, operators, additional images, and Helm charts. A global blocked images list can exclude specific images from any content type.

This document explains the filtering semantics for each content type and provides configuration examples.

## Platform / Release filtering

Release channels define which OCP or OKD versions to mirror.

### Channel selection

```yaml
mirror:
  platform:
    channels:
      - name: stable-4.18
```

### Version ranges

Use `minVersion` to constrain the range of releases mirrored within a channel:

```yaml
mirror:
  platform:
    channels:
      - name: stable-4.18
        minVersion: 4.18.1
```

Version range behavior:
- **Neither set:** Only the channel head (latest version) is mirrored (heads-only mode, the default).
- **Only `minVersion` set:** All versions from `minVersion` up to the channel head.
- **`full: true`:** Mirrors the entire channel regardless of version constraints.

**Note:** The `maxVersion` field is supported but **not recommended**. Using `maxVersion` can result in mirroring a version that is not the channel head, which may lack metadata required for proper cluster upgrades. Prefer using `minVersion` alone or `full: true` instead.

### Upgrades from 4.x to 5.x

When the configured OCP channels span two major versions, oc-mirror also mirrors the releases needed to upgrade from the lowest configured release to the highest one:

```yaml
mirror:
  platform:
    channels:
      - name: stable-4.22
        minVersion: 4.22.1
      - name: stable-5.0
```

oc-mirror first looks for the upgrade path in the target channel's graph (`stable-5.0`), which must contain the highest release. If that graph has no path, oc-mirror adds the source graph and searches the two graphs together. If neither search finds a path and other configured OCP channels have the same start or target version, for example `fast-5.0` and `stable-5.0` both ending at 5.0.1, oc-mirror tries those channels in a fixed order and uses the first path it finds. It does not look for cross-major links in channels that are not configured or that have other start and target versions.

In these examples, the graphs are made up, not published update paths:

| Case | Example graphs | Releases added for the upgrade |
|---|---|---|
| The target graph has a path | `stable-5.0`: 4.22.1 → 4.22.20 → 5.0.1 | 4.22.1, 4.22.20, 5.0.1 |
| The source graph adds patch updates | `stable-4.22`: 4.22.1 → 4.22.20; `stable-5.0`: 4.22.20 → 5.0.1 | 4.22.1, 4.22.20, 5.0.1 |
| Neither graph supplies a path | `stable-4.22`: 4.22.1 only; `stable-5.0`: 5.0.1 only | 4.22.1 and 5.0.1, with a `NoUpgradePath` warning |

When no path is found, oc-mirror logs a `NoUpgradePath` warning that lists the searched channels and the reason, and continues. For this upgrade, it adds only the start and target releases, or only the target release when the start release is in none of the searched graphs. The releases that each channel selects, for example with `minVersion`, are still mirrored.

oc-mirror only follows the unconditional update edges that the channels publish, never conditional update edges. When the start and target releases are generally available, the path skips pre-releases such as 5.0.0-rc.1, so a path that needs a pre-release between them ends with the warning, like a path that needs a conditional edge. Mirroring only the start and target releases does not confirm that a cluster can upgrade between them.

| Situation | Result |
|---|---|
| The target channel does not contain the highest release, a required graph cannot be downloaded or read, or a downloaded graph cannot be saved | The run fails |

Mirror to disk saves the searched graphs in the working directory, and disk to mirror uses this saved copy without contacting the update service. To use a newly published update path, run mirror to disk or mirror to mirror again.

### Architecture selection

By default, only `amd64` images are mirrored. Specify additional architectures at the platform level:

```yaml
mirror:
  platform:
    architectures:
      - amd64
      - arm64
      - ppc64le
      - s390x
      - multi
```

The `multi` option mirrors fat manifests (multi-architecture image indexes) containing all four architectures. This uses approximately 4x more registry space than a single architecture.

**Note:** The `architectures` field is deprecated starting with OpenShift 5 in favor of the `platforms` field.

### Graph data

Set `graph: true` to download and mirror the Cincinnati update graph data image. This is required for the OpenShift Update Service (OSUS) to calculate available upgrade paths on disconnected clusters:

```yaml
mirror:
  platform:
    graph: true
    channels:
      - name: stable-4.18
```

### OKD support

Use `type: okd` on a channel to mirror OKD releases instead of OCP:

```yaml
mirror:
  platform:
    channels:
      - name: 4-stable
        type: okd
```

## Operator filtering

Operator filtering controls which packages, channels, and bundle versions from a catalog are mirrored.

### Entire catalog (heads-only)

With no package filtering, oc-mirror mirrors the channel head of every channel for every package in the catalog:

```yaml
mirror:
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.18
```

### Full catalog

Mirror all bundles of all channels in the catalog:

```yaml
mirror:
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.18
      full: true
```

### Filter by package

Mirror only specific operator packages (channel head of every channel in each package):

```yaml
mirror:
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.18
      packages:
        - name: aws-load-balancer-operator
        - name: 3scale-operator
```

### Filter by package and channel

Mirror specific channels for a package. If the filtered channels do not include the catalog's original default channel, you **must** set `defaultChannel` to one of the included channels — otherwise oc-mirror will return an error:

```yaml
mirror:
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.18
      packages:
        - name: elasticsearch-operator
          defaultChannel: stable-v0
          channels:
            - name: stable-v0
```

### Filter by version range

Use `minVersion` to mirror bundles starting from a specific version up to the channel head:

```yaml
# Per-channel version range
mirror:
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.18
      packages:
        - name: elasticsearch-operator
          channels:
            - name: stable
              minVersion: 5.6.0

# Package-level version range (applies to all channels)
mirror:
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.18
      packages:
        - name: elasticsearch-operator
          minVersion: 5.6.0
```

**Important:** Specifying both package-level and channel-level version ranges is not allowed and will produce an error. However, you can specify channel names alongside a package-level version range (the range applies to each named channel). Similarly, `full: true` cannot be combined with version ranges.

**Note:** The `maxVersion` field is supported but **not recommended**. If the specified maximum version is not the channel head, the mirrored bundles may lack metadata required to display the operator correctly in the cluster.

### Target catalog overrides

Customize the destination path and tag for a mirrored catalog:

```yaml
mirror:
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.18
      targetCatalog: my-namespace/my-operator-index
      targetTag: latest
```

### OCI-based catalogs

File-based catalogs stored locally can be referenced with the `oci://` protocol:

```yaml
mirror:
  operators:
    - catalog: oci:///path/to/local/catalog
```

### Architecture selection for operators

Restrict an operator catalog to specific platforms with the `platforms` field:

```yaml
mirror:
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.22
      packages:
        - name: kubevirt-hyperconverged
          channels:
            - name: stable
      platforms:
        - os: linux
          architecture: amd64
```

A bundle's `relatedImages` holds both the images the operator runs and the
**operands** it deploys. They are treated differently, because the catalog
chooses that list, not you:

| image missing a requested platform | result |
|---|---|
| one the operator runs | mirroring fails: `operator "kubevirt-hyperconverged" does not support linux/ppc64le` |
| an operand | available requested platforms are mirrored; missing platforms are skipped with a warning. The whole operand is skipped only when none of the requested platforms are available |
| one you named in `additionalImages` or a release | mirroring fails: the platform is missing from your configuration |

Telling the two apart means reading the bundle's ClusterServiceVersion, so
oc-mirror does it only after a copy has already failed on a platform mismatch,
and caches the answer per bundle. A catalog whose images all mirror cleanly
pulls no bundles. Nothing is skipped unless it is proven to be an operand.

For example, when requesting `linux/amd64` and `linux/arm64`, an operand
manifest list containing only `linux/amd64` is still mirrored for `linux/amd64`.
The missing `linux/arm64` is skipped without pulling unrequested architectures.
Bundles and images the operator runs are never skipped this way: a missing
requested platform remains an error.

OLM's `operatorframework.io/arch.<arch>` labels are deliberately not used:
setting them is optional, so their absence proves nothing and a stale value
would block a mirror that works.

In mirror-to-disk, skipped operands and partially mirrored operands' available
platforms are recorded in the archive so disk-to-mirror makes the same decision
without accessing the source registry.

## Additional images

Individual container images can be mirrored by specifying their full reference. If no tag is specified, `latest` is assumed:

```yaml
mirror:
  additionalImages:
    - name: quay.io/example/my-image:v1.0
    - name: registry.redhat.io/ubi8/ubi@sha256:abc123...
```

Target overrides are available for additional images as well:

```yaml
mirror:
  additionalImages:
    - name: quay.io/example/my-image:v1.0
      targetRepo: custom-namespace/my-image
      targetTag: custom-tag
```

## Helm chart filtering

### Remote repositories

```yaml
mirror:
  helm:
    repositories:
      - name: podinfo
        url: https://stefanprodan.github.io/podinfo
        charts:
          - name: podinfo
            version: 5.0.0
```

If no specific charts are listed, all charts in the repository are mirrored.

### Local charts

```yaml
mirror:
  helm:
    local:
      - name: my-chart
        path: /path/to/local/chart
```

### Custom image paths

For Helm charts that store image references in non-standard locations, use `imagePaths` with JSON path expressions:

```yaml
mirror:
  helm:
    repositories:
      - name: podinfo
        url: https://stefanprodan.github.io/podinfo
        charts:
          - name: podinfo
            version: 5.0.0
            imagePaths:
              - "{.spec.template.spec.custom[*].image}"
```

## Blocked images

Exclude images matching regex patterns from mirroring, regardless of content type:

```yaml
mirror:
  blockedImages:
    - name: ".*blocked-image.*"
    - name: "registry.example.com/unwanted/.*"
```

Blocked image patterns are applied after collection and before mirroring. Any image whose reference matches a blocked pattern will be excluded.

## Related documentation

- [Mirroring Workflows](mirroring-workflows.md) — How the three mirroring modes work
- [Archive Management](archive-management.md) — Archive segmentation and incremental mirroring
- [Delete Functionality](delete-functionality.md) — Deleting previously mirrored images
