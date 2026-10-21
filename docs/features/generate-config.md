# Generate configuration from a registry

`oc-mirror --v2 generate` reverse-engineers an `ImageSetConfiguration` and a
`DeleteImageSetConfiguration` from the content of an existing mirror registry.
It is useful when the original configuration files are lost.

## Usage

```bash
oc-mirror --v2 generate docker://registry.example.com:5000
oc-mirror --v2 generate docker://registry.example.com:5000 --prefix=mirror/ --output-type=disc --output-dir=./out
oc-mirror --v2 generate docker://registry.example.com:5000 --repos=openshift/release-images,redhat/redhat-operator-index
```

The registry argument must be `docker://host[:port]`. Use `--prefix` to restrict to a registry path.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--output-type` | `both` | Files to generate: `isc`, `disc` or `both` |
| `--output-dir` | `.` | Directory the files are written to |
| `--prefix` | empty | Only consider repositories starting with this registry path |
| `--repos` | empty | Comma-separated repositories to inspect, for registries where `/v2/_catalog` is disabled |

`--output-dir` must already exist.

For registries with a self-signed certificate or plain HTTP, use `--src-tls-verify=false`. For credentials, use `--authfile` to point to an auth file.

## What is inferred

- Releases: platform channels and versions found in the release repositories.
- Operators: catalogs and the packages found in them.
- Additional images: tagged images that are neither releases nor operator content.

## What is not inferred

- Helm charts.
- Original channel names.
- Original source locations (the registry content does not record them).
- Additional images that were mirrored by digest.

## Prefix and target fields

`--prefix` matches on path boundaries: `--prefix=team-a` matches `team-a/app` but not `team-ab/app`.

When a prefix is set, mirroring or deleting against `docker://host/<prefix>` appends the target path to the prefix. To avoid a doubled prefix, the generated entries set `targetRepo` (additional images) or `targetCatalog` (operators) to the repository path without the prefix. For example, with `--prefix=mirror` the repository `mirror/ubi9/ubi` produces:

```yaml
additionalImages:
  - name: registry.example.com:5000/mirror/ubi9/ubi:latest
    targetRepo: ubi9/ubi
```

so that `oc-mirror -c imageset-config.yaml docker://registry.example.com:5000/mirror` writes to `mirror/ubi9/ubi`. Without a prefix, no target fields are set.

## Warnings

Tags that look like digests are skipped and counted in a warning: `sha256-<hex>` tags and any 12-character hex tag. The 12-hex rule can give false positives, for example a timestamp-like tag such as `202401011230`. Signature tags (`.sig`, `.att`, `.sbom`) are skipped silently and are not counted. Warnings are logged and also written as header comments in the generated files.

If probing a repository for the operator catalog label fails, the repository is treated as additional images and a warning is emitted. OCI artifacts such as Helm charts are not catalogs.

Image references point at the mirror registry, not the original sources, but this is only partial: platform channels and the related images inside rebuilt operator catalogs still resolve to upstream registries when mirrored again.

## Registries without `/v2/_catalog`

Some registries disable the `/v2/_catalog` endpoint. Pass the repositories to inspect with `--repos`.

## Existing files

The command never overwrites existing files. If a target file exists, it fails.

## Review before use

The generated files are a best-effort reconstruction. Review them, especially
before using the `DeleteImageSetConfiguration` with `delete`, since it can remove content.
