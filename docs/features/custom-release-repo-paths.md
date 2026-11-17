# Custom Release Repository Paths

## Overview

By default, oc-mirror copies release images to `openshift/release-images` and
release component images to `openshift/release` on the destination registry.
The `releaseImageRepo` and `releaseComponentRepo` fields in the
ImageSetConfiguration allow you to override these destination paths, for
example to preserve the original source repository paths.

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
      maxVersion: 4.22.2
```

| Field | Default | Description |
|-------|---------|-------------|
| `releaseImageRepo` | `openshift/release-images` | Destination repository path for release images |
| `releaseComponentRepo` | `openshift/release` | Destination repository path for release component images |

Both fields are optional. When omitted, the default paths are used. Values
must be lowercase repository paths without a tag or digest. The graph image
produced by `graph: true` is not affected by these overrides; it is always
mirrored to its own `openshift/graph-image` path.

Use the same values in the mirrorToDisk and diskToMirror configurations, and
in the `DeleteImageSetConfiguration` when generating a delete: the
diskToMirror run looks up release images in the cache at the configured paths,
and the generated delete list points at them too.

Run oc-mirror:

```bash
oc-mirror --v2 \
  --authfile ./pull-secret.json \
  --config ./imageset-config.yaml \
  --workspace file:///home/user/oc-mirror-workspace \
  docker://registry.example.com/mirrors/quay.io
```

Each release in the range is copied to the overridden paths instead of the
defaults:

```text
registry.example.com/mirrors/quay.io/openshift/release-images:4.22.0-x86_64
registry.example.com/mirrors/quay.io/openshift/release:4.22.0-x86_64-<component>
```

becomes

```text
registry.example.com/mirrors/quay.io/openshift-release-dev/ocp-release:4.22.0-x86_64
registry.example.com/mirrors/quay.io/openshift-release-dev/ocp-v4.0-art-dev:4.22.0-x86_64-<component>
```

## IDMS Configuration

oc-mirror writes an `ImageDigestMirrorSet` to
`<workspace>/working-dir/cluster-resources/idms-oc-mirror.yaml`. Because the
mirrored paths end with the source paths, its entries are scoped to the
namespace (for example `quay.io/openshift-release-dev` →
`registry.example.com/mirrors/quay.io/openshift-release-dev`) rather than one
entry per repository.

If you already use a registry-wide rule like the one below, the preserved
paths make it apply to release content as well:

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
    # optional, for fully disconnected environments
    mirrorSourcePolicy: NeverContactSource
```

Such a rule only resolves images that exist on the mirror; with
`NeverContactSource`, pulls of any other `quay.io` image fail. Payloads
referenced from another registry, such as OKD payloads listed in the update
graph as `registry.ci.openshift.org/origin/release-scos@sha256:…`, are not
covered by a `quay.io` rule, so keep the generated IDMS for those.
