# Troubleshooting

## Verbose logging

Use the `--log-level=debug` flag (or `--log-level=trace` for maximum verbosity) to get detailed diagnostic output. Log levels available: `info` (default), `debug`, `trace`, `error`.

Logs are printed to standard out and also written to a timestamped log file under the working directory:

```
<workspace>/working-dir/logs/oc-mirror-<timestamp>.log
```

Attach this file when submitting bug reports.

## Content discovery

To troubleshoot issues with ImageSetConfiguration content selection, use the `list` subcommands to discover available content:

```bash
# List available release versions
oc-mirror list releases --version=4.18

# List available operator catalogs
oc-mirror list operators --catalogs --version=4.18

# List packages in a catalog
oc-mirror list operators --catalog=registry.redhat.io/redhat/redhat-operator-index:v4.18

# List channels for a package
oc-mirror list operators --catalog=registry.redhat.io/redhat/redhat-operator-index:v4.18 --package=kiali

# List versions in a channel
oc-mirror list operators --catalog=registry.redhat.io/redhat/redhat-operator-index:v4.18 --package=kiali --channel=stable
```

## Dry run

Use `--dry-run` to preview what would be mirrored without actually copying images. This produces a `mapping.txt` file listing all source-to-destination image mappings, and for mirror-to-disk workflows, a `missing.txt` file listing images not yet in the local cache. See [Dry Run](docs/features/dry-run.md) for details.

## Registry access issues

oc-mirror checks registry access before starting a mirror operation. If authentication fails:

1. Verify your credentials are present in `$XDG_RUNTIME_DIR/containers/auth.json` or `~/.docker/config.json`
2. Ensure your [Red Hat OpenShift Pull Secret](https://console.redhat.com/openshift/install/pull-secret) is merged into the credentials file
3. For registries with custom TLS certificates, add them to the system trust store

## Cache issues

oc-mirror uses a local cache at `~/.oc-mirror` (configurable via `--cache-dir` or the `OC_MIRROR_CACHE` environment variable). If the cache becomes corrupted:

```bash
# Remove the cache and re-run mirroring
rm -rf ~/.oc-mirror
oc-mirror -c ./isc.yaml file:///path/to/output
```

Use `--force` to re-copy all content regardless of cache/history state:

```bash
oc-mirror -c ./isc.yaml --force file:///path/to/output
```

## Operator installation

To troubleshoot issues with the created file-based catalog on a cluster, inspect the package manifests:

```bash
oc get packagemanifests -n openshift-marketplace <packagename> -o json | jq ‘.status.channels[]|{name: .name, currentCSV: .currentCSV}’
```

## Destination registry parsing

For docker registry destinations, to preserve the same docker reference format across the ecosystem, `docker://registry` is not parsed as a registry hostname but as an image or repository name. To specify a registry, qualify the hostname or use an IP address. For example, use `docker://registry.localdomain`. `docker://localhost` works as expected because localhost is treated as a special exception.

## Common errors

### `unable to get OCI Image from oci:///$LOCATION: more than one image in oci, choose an image`

The OCI catalog at `$LOCATION` contains more than one manifest, and oc-mirror cannot determine which to use. This usually happens if a catalog is copied to the same location more than once.

### `when destination is docker://, either --from or --workspace need to be provided`

When the destination uses the `docker://` prefix, you must specify either `--from file://<path>` (for disk-to-mirror) or `--workspace file://<path>` (for mirror-to-mirror). See [Mirroring Workflows](docs/features/mirroring-workflows.md) for details.

### `use the mandatory --config flag`

The `-c` / `--config` flag pointing to an ImageSetConfiguration file is required for all mirroring operations.