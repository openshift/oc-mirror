# Mirror OKD

## Signature Verification

To pass signature verification when mirroring OKD images, download the OKD
release signing key and point oc-mirror at it and at the OKD signature store:

```bash
curl -sfLo verifier-public-key-openshift-ci-4 \
  https://raw.githubusercontent.com/openshift/cluster-update-keys/master/keys/verifier-public-key-openshift-ci-4
export OCP_SIGNATURE_URL="https://storage.googleapis.com/openshift-ci-release/releases/signatures/openshift/release/"
export OCP_SIGNATURE_VERIFICATION_PK="$PWD/verifier-public-key-openshift-ci-4"
```

If the key file cannot be read, oc-mirror logs a warning and falls back to the
default Red Hat key, so verification then fails with `invalid signature`.

Alternatively, `--ignore-release-signature` skips verification of release
signatures. Release images are then tagged by digest instead of by version,
because the version tag is taken from the signature, so use the flag in both
the mirrorToDisk and the diskToMirror run or in neither.

## Basic Configuration

OKD channels follow a different naming convention than OCP. Releases are
listed at https://origin-release.ci.openshift.org, but note that the channel
names differ from the website headings:

| Website heading | Channel name |
|-----------------|--------------|
| 4-scos-stable | `stable-4-scos` |
| 4-scos-next | `next-4-scos` |
| 5-scos-stable | `stable-5-scos` |
| 5-scos-next | `next-5-scos` |

Set `type: okd` on each OKD channel.

```yaml
---
apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
mirror:
  platform:
    graph: false
    channels:
    - name: stable-4-scos
      minVersion: 4.18.0-okd-scos.8
      maxVersion: 4.18.0-okd-scos.10
      type: okd
```

`graph: false` is the default and is shown for clarity: the update graph data
it would mirror comes from the OpenShift Update Service and covers OCP
releases, not OKD.

## Custom Repository Paths

By default, OKD release images are copied to `openshift/release-images` and
component images to `openshift/release` on the destination registry. Use
`releaseImageRepo` and `releaseComponentRepo` to override these paths, for
example to match the source repository layout:

```yaml
mirror:
  platform:
    releaseImageRepo: "okd/scos-release"
    releaseComponentRepo: "okd/scos-content"
    channels:
    # ... as above
```

See [Custom Release Repository Paths](features/custom-release-repo-paths.md)
for details.

Run oc-mirror:

```bash
oc-mirror --v2 \
  --authfile ./pull-secret.json \
  --config ./imageset-config.yaml \
  --workspace file:///home/user/oc-mirror-workspace \
  docker://registry.example.com/mirrors/quay.io
```

With the overrides above, each release in the range is copied to the
overridden paths. For example, 4.18.0-okd-scos.8 is copied to:

```text
registry.example.com/mirrors/quay.io/okd/scos-release:4.18.0-okd-scos.8
registry.example.com/mirrors/quay.io/okd/scos-content:4.18.0-okd-scos.8-<component>
```
