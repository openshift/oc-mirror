# Custom release repository paths

The optional `platform.targetImageRepo` and `platform.targetComponentRepo` fields
let you configure release payload and component repositories independently.
They apply to direct registry mirroring, archive workflows, and generation of
deletion metadata.

| Field | Images affected | Default repository |
|-------|-----------------|--------------------|
| `targetImageRepo` | Release payloads | `openshift/release-images` |
| `targetComponentRepo` | Release components, including kubevirt images | `openshift/release` |

Each field is optional. Setting one does not change the other repository. You
can also set both fields to the same repository; conflicting destination tags
are rejected. Neither field changes image tags or the separate graph repository
used by `graph: true` (normally `openshift/graph-image`).

## Preserve upstream repository paths

For example, to mirror OCP releases under a registry-wide `quay.io` prefix:

```yaml
apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
mirror:
  platform:
    targetImageRepo: openshift-release-dev/ocp-release
    targetComponentRepo: openshift-release-dev/ocp-v4.0-art-dev
    channels:
      - name: stable-4.20
        minVersion: 4.20.8
        maxVersion: 4.20.9
```

```bash
oc-mirror --v2 --config imageset-config.yaml \
  --workspace file:///home/user/mirror-workspace \
  docker://registry.example.com/mirrors/quay.io
```

The repositories are relative to the destination registry and namespace. This
example places images at:

```text
registry.example.com/mirrors/quay.io/openshift-release-dev/ocp-release:<release-tag>
registry.example.com/mirrors/quay.io/openshift-release-dev/ocp-v4.0-art-dev:<release-tag>-<component>
```

Preserving both upstream paths allows a registry-wide mirror mapping from
`quay.io` to `registry.example.com/mirrors/quay.io` to cover both image types.
oc-mirror also generates IDMS/ITMS resources matching the actual destinations.
Keep those resources for sources from other registries, which a `quay.io`-only
mapping does not cover.

For OKD, the fields can similarly be set to `okd/scos-release` and
`okd/scos-content`. These are platform-level settings and apply to every selected
release channel, not separate settings per channel.

## Validation

Both values must be lowercase repository paths without a registry transport,
tag, digest, leading slash, or trailing slash. Validation also checks the full
repository name, including the destination registry and namespace, against the
container reference length limit.

If `--max-nested-paths` is set, explicit repository paths must fit that limit
including the destination namespace; oc-mirror rejects them rather than silently
flattening the requested paths. Default paths and the graph repository can
still be flattened by that flag. Either override is rejected if it conflicts
with the graph repository when `graph: true` is enabled.

## Archive workflows and deletion

Use the same values for **each field** in mirror-to-disk and disk-to-mirror
configurations. The values also determine cache paths; disk-to-mirror must be
able to find the repositories written by mirror-to-disk.

Use the same fields in the `DeleteImageSetConfiguration` when generating delete
metadata:

```yaml
apiVersion: mirror.openshift.io/v2alpha1
kind: DeleteImageSetConfiguration
delete:
  platform:
    targetImageRepo: openshift-release-dev/ocp-release
    targetComponentRepo: openshift-release-dev/ocp-v4.0-art-dev
    channels:
      - name: stable-4.20
        minVersion: 4.20.8
        maxVersion: 4.20.9
```

Existing limitations around graph deletion after nested-path rewriting and
signature deletion without cached images are separate from these repository
settings.

The earlier, unmerged `platform.targetRepo` proposal is replaced by these two
fields; it is not an alias. To use a shared repository, set both new fields to
that repository explicitly.
