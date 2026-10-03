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

### Filter related images by label

Operator authors can label the related images of a bundle, typically to say which product
feature each image belongs to. Use `selectors` on a package to mirror only the related
images you need, and leave the images of the features you do not use behind:

```yaml
mirror:
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.18
      packages:
        # mirror the images without labels, the ones for CoolFeatureA, and the ones for
        # GreatFeatureB that are version 1.2.3 and tier frontend
        - name: aws-load-balancer-operator
          selectors:
            - matchExpressions:
                - key: CoolFeatureA
                  operator: Exists
            - matchExpressions:
                - key: GreatFeatureB
                  operator: Exists
                - key: tier
                  operator: In
                  values:
                    - frontend
              matchLabels:
                version: "1.2.3"

        # mirror everything except the images for CoolFeatureC
        - name: 3scale-operator
          selectors:
            - matchExpressions:
                - key: CoolFeatureC
                  operator: DoesNotExist

        # no selectors: mirror only the images without labels (default)
        - name: node-observability-operator
```

A `selectors` entry is a [Kubernetes label selector](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors),
with the syntax and the semantics Kubernetes defines for it. A related image is mirrored when:

- it carries no label at all, **or**
- at least one selector of its package matches its labels.

The requirements within one selector are ANDed; the selectors of a package are ORed. A package
with no selector therefore mirrors only the related images that carry no label, which is also
what packages absent from the configuration and catalogs without any label do — so catalogs that
carry no label are mirrored exactly as before.

Selection is per package: an image left out for one package is still mirrored when another
package selects it. Every image left out is logged, so you can audit what a run skipped:

```text
image registry.redhat.io/feature-c:v1.2.3 is not mirrored in bundle 3scale-operator.v1.2.3: its labels map[CoolFeatureC:] are selected by no selector of operator 3scale-operator
```

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
