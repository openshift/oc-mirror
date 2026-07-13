# Mirror OKD

## Signature Verification

To pass signature verification when mirroring OKD images, set the following environment variables:

```bash
export OCP_SIGNATURE_URL="https://storage.googleapis.com/openshift-ci-release/releases/signatures/openshift/release/"
export OCP_SIGNATURE_VERIFICATION_PK="/path/to/verifier-public-key-openshift-ci-4"
```

The public key can be retrieved from https://raw.githubusercontent.com/openshift/cluster-update-keys/master/keys/verifier-public-key-openshift-ci-4

Alternatively, use `--ignore-release-signature` to skip release signature verification.

## Basic Configuration

OKD channels follow a different naming convention than OCP. Releases are listed at https://origin-release.ci.openshift.org, but note that the channel names differ from the website headings:

| Website heading | Channel name |
|-----------------|--------------|
| 4-scos-stable | `stable-4-scos` |
| 4-scos-next | `next-4-scos` |
| 5-scos-next | `next-5-scos` |

The `type: okd` field must be set.

```yaml
---
apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
mirror:
  platform:
    channels:
    - name: stable-4-scos
      minVersion: 4.18.0-okd-scos.8
      maxVersion: 4.18.0-okd-scos.8
      type: okd
```

## Custom Repository Paths

By default, OKD release images are copied to `openshift/release-images` and component images to `openshift/release` on the destination registry. Use `releaseImageRepo` and `releaseComponentRepo` to override these paths, for example to match the source repository layout:

```yaml
mirror:
  platform:
    releaseImageRepo: "okd/scos-release"
    releaseComponentRepo: "okd/scos-content"
    channels:
      ...
```

Run oc-mirror:

```bash
oc-mirror --v2 \
  --authfile ./pull-secret.json \
  --config ./imageset-config.yaml \
  --workspace file:///home/user/oc-mirror-workspace \
  docker://registry.example.com/mirrors/quay.io
```

With the overrides above, images are copied to:

```
registry.example.com/mirrors/quay.io/okd/scos-release:4.18.0-okd-scos.8
registry.example.com/mirrors/quay.io/okd/scos-content:4.18.0-okd-scos.8-<component>
```
