# oc-mirror
- [oc-mirror](#oc-mirror)
  - [Community](#community)
  - [Usage](#usage)
    - [Configuration Examples](#configuration-examples)
    - [Environment Prep](#environment-prep)
    - [Building the ImageSet Config](#building-the-imageset-config)
      - [Backends](#backends)
    - [Content Discovery](#content-discovery)
      - [Updates](#updates)
      - [Releases](#releases)
      - [Operators](#operators)
  - [oc-mirror Spec](#oc-mirror-spec)
  - [Development](#development)
    - [Requirements](#requirements)
    - [Build](#build)
    - [Test](#test)

`oc-mirror` is an OpenShift Client (oc) plugin that manages OpenShift release, operator catalog, helm charts, and associated container images for mirror registries that support OpenShift environments.

## Community
Please join us for our oc-mirror weekly community meeting fortnightly on Thursdays at 12pm EST https://zoom.us/meeting/tJ0lcOiqpzMpE9OQ8gwJktuKL7ZWAr3Ofe4s/ics?icsToken=98tyKuChpjwrGN2dsxCARox5Go_Cd-rxmFxej_pyrUfiFApYdhbdGrV4BZNtQ_fW .  
 
## Usage
[![asciicast](https://asciinema.org/a/uToc11VnzG0RMZrht2dsaTfo9.svg)](https://asciinema.org/a/uToc11VnzG0RMZrht2dsaTfo9)

The mirror registry `reg.mirror.com` is used in this example.
Replace this value with a real registry host, or create a `docker.io/library/registry:2` container locally.\

> DISCLAIMER: `oc-mirror` is not compatible with Quay below version 3.6.

### Configuration Examples

Example configurations can be found in the docs [here](docs/examples)
### Environment Prep
1. Download pull secret and place at `~/.docker/config.json`<sup>1</sup>.
    - Your mirror registry secret must have both push and pull scopes.
2. Build:
    ```sh
    make build
    ```
### Building the ImageSet Config
#### Backends
> **IMPORTANT**: Backends must be configured to utilize the lifecycle management features of `oc-mirror`. Examples are below.
```sh
apiVersion: mirror.openshift.io/v1alpha2
kind: ImageSetConfiguration
archiveSize: 1
storageConfig:
  local:
    path: /home/user/workspace
```
```sh
apiVersion: mirror.openshift.io/v1alpha2
kind: ImageSetConfiguration
storageConfig:
  registry:
    imageURL: localhost:5000/metadata:latest
    skipTLS: true
```
### Lifecycle Management

There is a common misconception that oc-mirror is only a container image mirroring utility. This is partially accurate. In addition to image mirroring, oc-mirror optionally performs content deletions from mirror registries. When content is added to an imageset configuration, that content is published to a mirror registry. When content is removed from an imageset configuration, that content is removed from the mirror registry. This ensures that content is safely removed from the mirror registry when the user specifies. 

If a user does not desire content removal from a mirror registry, they can either not remove that content from their imageset configuration or they can publish using a registry account that does not have permissions to perform content deletions. If a user chooses to disallow pruning using an account without permission to perform deletions, the `--continue-on-error` flag must be used in conjunction with the publishing operation to the mirror registry.

### Content Discovery

#### Updates

- List updates since the last `oc-mirror` run
  ```sh
  ./bin/oc-mirror list updates imageset-config.yaml
  ```
#### Releases
1. List all available release payloads for a version of OpenShift (defaults to stable)
   ```sh
   ./bin/oc-mirror list releases --version=4.9
   ```
2. List all available channels to query for a version of OpenShift
   ```sh
   ./bin/oc-mirror list releases --channels --version=4.9
   ```
3. List all available release payloads for a version of OpenShift in a specified channel
   ```sh
   ./bin/oc-mirror list releases --channel=fast-4.9
   ```
#### Operators
1. List all available catalogs for a version of OpenShift
   ```sh
   ./bin/oc-mirror list operators --catalogs --version=4.9
   ```
2. List all available packages in a catalog
   ```sh
   ./bin/oc-mirror list operators --catalog=catalog-name
   ````
3. List all available channels in a package
    ```sh
    ./bin/oc-mirror list operators --catalog=catalog-name --package=package-name
    ```
4. List all available versions in a channel
      ```sh
    ./bin/oc-mirror list operators --catalog=catalog-name --package=package-name --channel=channel-name
    ```
### Mirroring

**Note:** These workflows are compatible and can be used interchangeably for the same mirror registry.

**Note:** oc-mirror metadata is always stored where specified by the user and in an expected location managed by oc-mirror (always "registry/namespace/oc-mirror:UUID). This allows a user to seamlessly switch between a fully-disconnected workflow and a partially disconnected workflow.

#### Fully Disconnected
- Create then publish to your mirror registry:
    ```sh
    ./bin/oc-mirror --config imageset-config.yaml file://archives
    ./bin/oc-mirror --from /path/to/archives docker://reg.mirror.com
    ```
#### Partially Disconnected
- Publish mirror to mirror
     ```sh
    ./bin/oc-mirror --config imageset-config.yaml docker://localhost:5000
    ```
## Additional Features
- Get information on your imageset using `describe`
    ```sh
    ./bin/oc-mirror describe /path/to/archives
    ```
- List updates since last run for releases and operators
  ```sh
  ./bin/oc-mirror list updates --config imageset-config.yaml
  ```
For configuration and options, see the [expanded overview](./docs/overview.md) and [usage](./docs/usage.md) docs.

<sup>1</sup> For this example, the `create` and `publish` steps are run on the same machine. Therefore your `~/.docker/config.json` or `${XDG_RUNTIME_DIR}/containers/auth.json` should contain auth config for both release/catalog source images _and_ your mirror registry.

## oc-mirror Spec

See the [config spec][config-spec] for an in-depth description of fields.

**Note:** The `imageset-config.yaml` is only used during bundle creation.

## Development

### Requirements

- All top-level requirements
- [`go`][go] version 1.17+

### Build

```sh
make
./bin/oc-mirror -h
```

**Note:** Using `oc-mirror init` with local builds may result in unexpected release channel names.

### Test

Unit:
```sh
make test-unit
```

E2E:
```sh
make test-e2e
```

[config-spec]:https://pkg.go.dev/github.com/openshift/oc-mirror/pkg/config/v1alpha1#ImageSetConfiguration
[go]:https://golang.org/dl/

### Also:
oc-mirror is awesome
