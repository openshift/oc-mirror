# Generate ImageSetConfiguration from an existing mirror registry

## Goal

Given an existing mirror registry, emit an `ImageSetConfiguration` and/or a
`DeleteImageSetConfiguration` that describes what the registry holds
(reverse-engineering). Use cases: reproduce a mirror, or prune one.

## Non-goals

- Recovering the user's original intent. Output is best effort.
- Recovering Helm charts. oc-mirror mirrors the images a chart references, not
  the chart, so those images surface as `additionalImages`.

## CLI

`oc-mirror --v2 generate <registry-url>` (new subcommand, registered next to
`delete` and `list` in `internal/pkg/cli/executor.go`).

Flags:
- `--output-type isc|disc|both` (default `both`)
- `--output-dir` (default current dir)
- `--repos` explicit repo paths, used when `/v2/_catalog` is disabled
- `--prefix` restrict discovery to a registry path

Auth and TLS reuse `mirror.CopyOptions` (src flags).

## Components (`internal/pkg/generate`)

- `Lister`: enumerates repos (`_catalog` or `--repos`) and tags per repo, via
  the container-libs `docker` transport.
- `Classifier`: assigns each repo to release, operator catalog, or additional.
  - Release: `openshift/release-images` tags `<ver>-<arch>` give versions and
    architectures. Channel is derived as `stable-X.Y`; `minVersion` and
    `maxVersion` come from the tags found.
  - Operator: catalog repos and tags; packages, channels, versions read from
    the (filtered FBC) catalog image.
  - Additional: all remaining repos and tags, pinned by tag or digest.
- `Builder`: fills `v2alpha1.Mirror` or `v2alpha1.Delete` and marshals YAML
  with the right `kind` and `apiVersion`.

Data flow: registry -> Lister -> Classifier -> Builder -> YAML files
(`imageset-config.yaml`, `delete-imageset-config.yaml`).

## Errors

- Unreachable registry, or `_catalog` disabled with no `--repos`: fail with a
  hint.
- Unclassifiable repos: go to `additionalImages`, logged.
- Lossy inference (derived channels, Helm): YAML comment plus log warning.

## Testing

- Unit tests with a fake Lister; table-driven Classifier cases asserting exact
  YAML.
- Round trip: generated YAML must parse and validate through the existing
  config code.
- Integration test against a local registry if the repo pattern supports it.

## Docs

`docs/features/generate-config.md`.
