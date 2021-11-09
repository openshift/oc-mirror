# oc-mirror

**This repo is under active development. CLI and APIs are unstable**

oc-mirror is an OpenShift Client (oc) plugin that manages OpenShift release, operator catalog, helm charts, and associated container images.

oc-mirror management is a two part process:
1. oc-mirror Creation (Internet Connected)
1. oc-mirror Publishing (Disconnected)

## Usage

The mirror registry `reg.mirror.com` is used in this example.
Replace this value with a real registry host, or create a `docker.io/library/registry:2` container locally.

1. Download pull secret and place at `~/.docker/config.json`<sup>1</sup>.
    - Your mirror registry secret must have both push and pull scopes.
1. Build:
    ```sh
    make build
    ```
1. Create then publish to your mirror registry:
    ```sh
    ./bin/oc-mirror --config imageset-config.yaml --dir test-create file://archives
    ./bin/oc-mirror --from archives --dir test-publish docker://reg.mirror.com
    ```

For configuration and options, see the [expanded overview](./docs/overview.md) and [usage](./docs/usage.md) docs.

<sup>1</sup> For this example, the `create` and `publish` steps are run on the same machine. Therefore your `~/.docker/config.json`
should contain auth config for both release/catalog source images _and_ your mirror registry.

## oc-mirror Spec

See the [config spec][config-spec] for an in-depth description of fields.

**Note:** The `imageset-config.yaml` is only used during bundle creation.

## Development

### Requirements

- All top-level requirements
- [`go`][go] version 1.16+

### Build

```sh
make
./bin/oc-mirror -h
```

### Test

Unit:
```sh
make test-unit
```

E2E:
```sh
make test-e2e
```

<!--
TODO: link to the following once a release is cut.
[config-spec]:https://pkg.go.dev/github.com/redhatgov/bundle/pkg/config/v1alpha1#ImageSetConfiguration
-->
[config-spec]:pkg/config/v1alpha1/config_types.go
[go]:https://golang.org/dl/
[docker-buildx]:https://docs.docker.com/buildx/working-with-buildx/
[podman]:https://podman.io/getting-started/
