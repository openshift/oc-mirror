# Custom Release Repository Paths

## Overview

By default, oc-mirror copies release images to `openshift/release-images` and release component images to `openshift/release` on the destination registry. The `releaseImageRepo` and `releaseComponentRepo` fields in the ImageSetConfiguration allow you to override these destination paths, for example to preserve the original source repository paths.

## Configuration

Add the fields to the `platform` section of your ImageSetConfiguration:

```yaml
apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
mirror:
  platform:
    releaseImageRepo: "openshift-release-dev/ocp-release"
    releaseComponentRepo: "openshift-release-dev/ocp-v4.0-art-dev"
    channels:
    - name: stable-4.22
      minVersion: 4.22.0
      maxVersion: 4.22.0
```

| Field | Default | Description |
|-------|---------|-------------|
| `releaseImageRepo` | `openshift/release-images` | Destination repository path for release images |
| `releaseComponentRepo` | `openshift/release` | Destination repository path for release component images |

Both fields are optional. When omitted, the default paths are used.

## Example

Given the following `imageset-config.yaml`:

```yaml
apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
mirror:
  platform:
    releaseImageRepo: "openshift-release-dev/ocp-release"
    releaseComponentRepo: "openshift-release-dev/ocp-v4.0-art-dev"
    channels:
    - name: stable-4.22
      minVersion: 4.22.0
      maxVersion: 4.22.0
```

Run oc-mirror:

```bash
oc-mirror --v2 \
  --authfile ./pull-secret.json \
  --config ./isc.yaml \
  --workspace file:///home/user/oc-mirror-workspace \
  docker://registry.example.com/mirrors/quay.io
```

With the default configuration, release images would be copied to:

```text
registry.example.com/mirrors/quay.io/openshift/release-images:4.22.0-x86_64
registry.example.com/mirrors/quay.io/openshift/release:4.22.0-x86_64-<component>
```

With the overrides above, they are instead copied to:

```text
registry.example.com/mirrors/quay.io/openshift-release-dev/ocp-release:4.22.0-x86_64
registry.example.com/mirrors/quay.io/openshift-release-dev/ocp-v4.0-art-dev:4.22.0-x86_64-<component>
```

## IDMS Configuration

With the original source paths preserved on the mirror, a single `ImageDigestMirrorSet` entry can cover all of `quay.io`:

```yaml
apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: quay-io-mirror
spec:
  imageDigestMirrors:
  - source: quay.io
    mirrors:
    - registry.example.com/mirrors/quay.io
    mirrorSourcePolicy: NeverContactSource  # optional, for fully disconnected environments
```

Because the mirrored paths match the source paths, every image under `quay.io/` is automatically resolved to `registry.example.com/mirrors/quay.io/` with no per-repository mappings needed.
