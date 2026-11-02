# Test images

Sources of the container images that the integration tests mirror from.

At the start of each test session, the suite starts a local registry on
`localhost:5001` (the *source registry*), builds every image from this directory
and pushes it there (`testimages.BuildAndPush`, in `pkg/testimages`). Tests and
test catalogs reference these images as `localhost:5001/<repository>:<tag>`.

Nothing is published to a remote registry: a change to these sources is tested by
the same pull request that introduces it.

The source registry serves HTTPS with a certificate signed by a CA generated for
the session. oc-mirror trusts it through
`$HOME/.config/containers/certs.d/localhost:5001/ca.crt`, so tests don't need
`--src-tls-verify=false` to pull from it.

Go never routes traffic to `localhost` through a proxy, so proxy tests reach the
source registry and the Helm repository through the `source.oc-mirror.test` and
`charts.oc-mirror.test` aliases, which only the test proxy resolves.

Every image (and every manifest of multi-arch images) is signed with
`testdata/keys/cosign.key`, as `cosign sign --key cosign.key --tlog-upload=false`
does: the signature is pushed as the `sha256-<digest>.sig` tag of the same repository.
Images meant to be unsigned are the exception. To regenerate the key pair, run
`testdata/keys/generate-cosign-keys.sh`.

Release payloads are also signed with a GPG key generated for the session, in the
format of OpenShift release signatures. The suite points
`OCP_SIGNATURE_VERIFICATION_PK` at its public key, and `setupWorkDir` copies the
signatures into `working-dir/signatures`.

## Contents

Operator images are pushed to the `oc-mirror/oc-mirror-dev` repository:

| Source | Tag |
|--------|-----|
| [operator/bundles](operator/bundles)`/<package>/<package>-bundle-v<version>` | `<package>-bundle-v<version>`: registry+v1 bundle. Labels come from `metadata/annotations.yaml`. |
| | `<package>-bundle-v<version>-multi`: linux/amd64 + linux/arm64 index, for the bundles listed in `multiArchBundles` (`pkg/testimages/operator.go`). |
| | `<package>-v<version>`: minimal operand, one per bundle. |
| | `operand`, `operand-alt`: the operands referenced by digest from bundles and catalogs. |
| [operator/catalogs](operator/catalogs)`/<catalog>` | `<catalog>`: file-based catalog served from `/configs`, rendered with opm (see its README). |

Release images are pushed under `oc-mirror/release` (see `releasePayloads` in
`pkg/testimages/release.go`):

| Source | Image |
|--------|-------|
| [release/test-release-index](release/test-release-index) | `test-release-index:v0.0.1`: linux/amd64 payload whose layer holds `release-manifests/`. |
| | `test-image:v0.0.1`: its component (linux/amd64). |
| [release/test-release-index-multi](release/test-release-index-multi) | `test-release-index-multi:v0.0.1`: payload index for amd64, arm64, s390x and ppc64le. |
| | `test-image-multi:v0.0.1`: its component (amd64 and arm64 index). |

Synthetic images, whose content is irrelevant, reproduce manifest structures that
some tests rely on (see `additionalImages` in `pkg/testimages/additional.go`):

| Image | Structure |
|-------|-----------|
| `oc-mirror/multi-platform-image:latest` | OCI index: amd64, arm64, s390x, ppc64le. |
| `oc-mirror/single-manifest-image:latest` | Single OCI manifest. |
| `unsigned/docker-manifest-list:latest` | Docker manifest list (arm64, ppc64le), not signed. |
| `oc-mirror/oc-mirror-dev:multilayers-latest` | Docker manifest with 15 layers. |
| `oc-mirror/test-chart:1.0.0` | Docker manifest list (amd64, arm64, arm/v7), used by the `test-chart` Helm chart. |

## Helm charts

The charts under `testdata/helm-charts` are served by a local Helm repository at
`http://localhost:5002` (`pkg/helmrepo`), packaged and indexed with the Helm SDK.
They are rendered as Go templates first (see below), so they must not use Helm
template actions.

## Referencing images by digest

Image digests are only known once the images are built. Bundle manifests, release
manifests, catalog FBC files, ImageSetConfigurations under `testdata/imagesetconfigs`
and Helm charts under `testdata/helm-charts` are rendered as Go templates and can use:

```yaml
image: localhost:5001/oc-mirror/oc-mirror-dev@{{ digest "oc-mirror/oc-mirror-dev:operand" }}
```

From Go tests, use `sourceImageDigest("oc-mirror/oc-mirror-dev:operand")`,
`sourceImageRef(...)` and `sourceImageDigestRef(...)` (see `integration_suite_test.go`).
