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

Use `minVersion` and `maxVersion` to constrain the range of releases mirrored within a channel:

```yaml
mirror:
  platform:
    channels:
      - name: stable-4.18
        minVersion: 4.18.1
        maxVersion: 4.18.5
```

Version range behavior:
- **Neither set:** Only the channel head (latest version) is mirrored (heads-only mode, the default).
- **Only `minVersion` set:** All versions from `minVersion` up to the channel head.
- **Only `maxVersion` set:** All versions from the oldest in the channel up to `maxVersion`.
- **Both set:** All versions between `minVersion` and `maxVersion`, inclusive.
- **`full: true`:** Mirrors the entire channel regardless of version constraints.

### Shortest path mode

When `shortestPath: true` is set, oc-mirror calculates the shortest upgrade path between `minVersion` and `maxVersion` using the Cincinnati graph, rather than including all versions in the range:

```yaml
mirror:
  platform:
    channels:
      - name: stable-4.18
        minVersion: 4.18.1
        maxVersion: 4.18.5
        shortestPath: true
```

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

With no package filtering, oc-mirror mirrors only the channel head (latest version) of each package's default channel:

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

Mirror only specific operator packages (channel head of each):

```yaml
mirror:
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.18
      packages:
        - name: aws-load-balancer-operator
        - name: 3scale-operator
```

### Filter by package and channel

Mirror specific channels for a package. Use `defaultChannel` if the filtered channels do not include the catalog's default channel:

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

Version ranges can be specified at the package level (applies to all channels) or per channel:

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
              maxVersion: 6.0.0

# Package-level version range (cannot be combined with channel filtering)
mirror:
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.18
      packages:
        - name: elasticsearch-operator
          minVersion: 5.6.0
          maxVersion: 6.0.0
```

**Important:** Specifying both package-level `minVersion`/`maxVersion` and channel-level filtering is not allowed and will produce an error. Similarly, `full: true` cannot be combined with version ranges.

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

### Skip dependencies

By default, oc-mirror includes operator bundle dependencies. Set `skipDependencies: true` to mirror only the explicitly specified bundles:

```yaml
mirror:
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.18
      skipDependencies: true
      packages:
        - name: my-operator
```

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
