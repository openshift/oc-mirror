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

## Warnings

Tags that look like digests (for example `sha256-<hex>` or signature tags) are skipped and counted in a warning. Warnings are logged and also written as header comments in the generated files.

## Registries without `/v2/_catalog`

Some registries disable the `/v2/_catalog` endpoint. Pass the repositories to inspect with `--repos`.

## Existing files

The command never overwrites existing files. If a target file exists, it fails.

## Review before use

The generated files are a best-effort reconstruction. Review them, especially
before using the `DeleteImageSetConfiguration` with `delete`, since it can remove content.
