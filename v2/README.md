# Overview

`oc-mirror` is an OpenShift Client (oc) plugin that mirrors OpenShift release, operator catalog and additional containers images. 

The command line uses a declarative configuration file as the input to discover from where to get the container images and copy them to the destination specified in the command line. 

## Table of contents
* [Getting Started](#getting-started)
  * [Prerequisites](#prerequisites)
  * [Download the repository](#download-the-repository)
  * [Building the oc-mirror binary](#building-the-oc-mirror-binary)
* [Using oc-mirror](#using-oc-mirror)
  * [Environment Preparation](#environment-preparation)
  * [Building the Image Set Configuration](#building-the-image-set-configuration)
  * [Workflows](#workflows)
    * [Mirror To Disk](#mirror-to-disk)
    * [Disk To Mirror](#disk-to-mirror)
    * [Mirror To Mirror](#mirror-to-mirror)
  * [Delete Feature](#delete-feature)
* [Development](#development)
  * [Architecture](#architecture)
  * [Image Set Configuration Specs](#image-set-configuration-specs)
* [Testing](#testing)
* [Find out more](#find-out-more)

## Getting Started
These instructions will provide a copy of the go module. After following these steps it will be possible to start using oc-mirror.

### Prerequisites
```
- Git
- Golang 1.19+
```

### Download the repository
```
git clone https://github.com/openshift/oc-mirror.git
```

### Building the oc-mirror binary
After cloning the oc-mirror repo, on the oc-mirror directory run the following command:
```
make build
```

This is going to generate the oc-mirror binary on `oc-mirror/bin/`.

## Using oc-mirror
The instructions in this section will show how to prepare the environment and use oc-mirror.

### Environment Preparation

**Setting up the credentials for pulling/pushing container images:** <br>
In order to be able to pull the containers images from the container registry, it is necessary to download the pull secret and place it in the correct path. 

The default podman credentials location `($XDG_RUNTIME_DIR/containers/auth)` is used for authenticating to the registries. The docker location `~/.docker/config.json` for credentials is also supported.


**Running a local registry to be used as a destination (Optional Step):**
```
podman run -dt -p 6000:5000 -e REGISTRY_STORAGE_DELETE_ENABLED=true --name local-registry docker.io/library/registry:2
```

### Building the Image Set Configuration
The Image Set Configuration is the input file used by oc-mirror to discover which images are necessary to be mirrored. 

Below is an example which included an Openshift Release (version 4.13.10), an operator catalog with only 3 operators and some additional images (ubi8 and ubi9):

```
kind: ImageSetConfiguration
apiVersion: mirror.openshift.io/v2alpha1
mirror:
  platform:
    channels:
    - name: stable-4.13
      minVersion: 4.13.10
      maxVersion: 4.13.10
    graph: true
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.15
      packages:
       - name: aws-load-balancer-operator
       - name: 3scale-operator
       - name: node-observability-operator
  additionalImages: 
   - name: registry.redhat.io/ubi8/ubi:latest
   - name: registry.redhat.io/ubi9/ubi@sha256:20f695d2a91352d4eaa25107535126727b5945bff38ed36a3e59590f495046f0
```
More Image Set Configuration example can be found [here]().

### Workflows
This section will explain all the workflows supported by oc-mirror currently.

#### Mirror To Disk
Pulls the container images from the source specified in the image set configuration and pack them into a tar archive on disk (local directory).
```
oc-mirror -c ./isc.yaml file:///home/<user>/oc-mirror/mirror1 --v2
```

#### Disk To Mirror
Copy the containers images from the tar archive to a container registry (--from flag is required on this workflow).
```
oc-mirror -c ./isc.yaml --from file:///home/<user>/oc-mirror/mirror1 docker://localhost:6000 --v2
```

#### Mirror To Mirror
Copy the container images from the source specified in the image set configuration to the destination (container registry).
```
oc-mirror -c ./isc.yaml --workspace file:///home/<user>/oc-mirror/mirror1 docker://localhost:6000 --v2
```

### Delete Feature
There is also a delete command to delete images in a destination. This command is split in two phases:

- Phase 1 (--generate): using a delete set configuration as an input, oc-mirror discovers all images that needed to be deleted. These images are included in a delete-images file to be consumed as input in the second phase.
```
oc-mirror delete -c ./delete-isc.yaml --generate --workspace file:///home/<user>/oc-mirror/delete1 --delete-id delete1-test docker://localhost:6000 --v2
```

- Phase 2: using the file generated in first phase, oc-mirror will delete all manifests specified on this file on the destination specified in the command line. It is up to the container registry to run the garbage collector to clean up all the blobs which are not referenced by a manifest. Deleting only manifests is safer since blobs shared between more than one image are not going to be deleted.
```
oc-mirror delete --delete-yaml-file /home/<user>/oc-mirror/delete1/working-dir/delete/delete-images-delete1-test.yaml docker://localhost:6000 --v2
```

## Development
This section will give an overview about oc-mirror v2 architecture. Pull requests are always welcome in the oc-mirror repository.

### Architecture
![Alt text](./assets/architecture.png)

### Image Set Configuration Specs
[Here](https://github.com/openshift/oc-mirror/blob/e7889a7ec70dd66b0d6a7ba6dedc3e4b93ebf4de/v2/pkg/api/v2alpha1/type_config.go#L16) that shows details about the specification of the Image Set Configuration with all available fields.

## Testing
Currently oc-mirror has unit tests and integration tests. 

To run unit test:

```
make test
```

To run integration tests:
```
make test-integration
```

## Find out more

- [delete feature](./docs/delete-functionality.md)
- [enclave support feature](./docs/enclave_support.md)
- [signature verification](./docs/signature-verification.md)
- [operator filtering investigation](./docs/operator-filtering-investigation.md)
- [progress bar and concurrency investigation](./docs/progress-bar-and-concurrencty-investigation.md)