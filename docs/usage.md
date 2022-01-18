# Usage

- [Usage](#usage)
  - [Overview](#overview)
  - [Prerequisites](#prerequisites)
    - [Authentication:](#authentication)
    - [Certificate Trust](#certificate-trust)
  - [Basic Usage](#basic-usage)
    - [Content Discovery](#content-discovery)
      - [Updates](#updates)
      - [Releases](#releases)
      - [Operators](#operators)
    - [Mirroring](#mirroring)
      - [Fully Disconnected](#fully-disconnected)
      - [Partially Disconnected](#partially-disconnected)
    - [Additional Features](#additional-features)
  - [Mirroring Process](#mirroring-process)
    - [Running `oc-mirror` For First Time](#running-oc-mirror-for-first-time)
    - [Running `oc-mirror` For Differential Updates](#running-oc-mirror-for-differential-updates)
  - [Glossary](#glossary)

## Overview

**Notice:** These commands are early alpha and may change significantly between application versions. 

## Prerequisites
> **WARNING**: Depending on the configuration file used and the periodicity between running `oc-mirror`, this process may download multiple hundreds of gigabytes of data, though differential updates should usually result in significantly smaller Imagesets.
### Authentication: 
oc-mirror currently retrieves registry credentials from `~/.docker/config.json` or `${XDG_RUNTIME_DIR}/containers/auth.json`. Make sure that your [Red Hat OpenShift Pull Secret](https://console.redhat.com/openshift/install/pull-secret) and any other needed registry credentials are populated in the credentials file.

### Certificate Trust

oc-mirror currently references the host system for certificate trust information. For now, you must [add all certificates (trust chain) to be trusted to the System-Wide Trust Store](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/7/html/security_guide/sec-shared-system-certificates)


## Basic Usage
### Content Discovery

oc-mirror provides a way to discover OpenShift release and operator content,
then use that information to craft mirror payloads. The `list updates` command traverses update graphs
between the last `oc mirror` run and provided configuration to show what new versions are available.

#### Updates

- List updates since the last `oc-mirror` run
  ```sh
  oc-mirror list updates --config imageset-config.yaml
  ```
**Note:** You must have existing metadata in your workspace (or remote storage, if using) to use `list updates`
#### Releases
1. List all available release payloads for a version of OpenShift in the stable channel (the default channel)
   ```sh
   oc-mirror list releases --version=4.9
   ```
2. List all available release channels to query for a version of OpenShift
   ```sh
   oc-mirror list releases --channels --version=4.8
   ```
3. List all available release payloads for a version of OpenShift in a specific channel
   ```sh
   oc-mirror list releases --channel=fast-4.9
   ```
#### Operators
1. List all available Operator catalogs for a version of OpenShift
   ```sh
   oc-mirror list operators --catalogs --version=4.9
   ```
2. List all available Operator packages in a catalog
   ```sh
   oc-mirror list operators --catalog=registry.redhat.io/redhat/redhat-operator-index:v4.9
   ````
3. List all available Operator channels in a package
    ```sh
    oc-mirror list operators --catalog=registry.redhat.io/redhat/redhat-operator-index:v4.9 --package=kiali
    ```
4. List all available Operator versions in a channel
      ```sh
    oc-mirror list operators --catalog=registry.redhat.io/redhat/redhat-operator-index:v4.9 --package=kiali --channel=stable
    ```
### Mirroring
#### Fully Disconnected
- Create then publish to your mirror registry:
    ```sh
    oc-mirror --config imageset-config.yaml file://archives
    oc-mirror --from /path/to/archives docker://reg.mirror.com
    ```
#### Partially Disconnected
- Publish mirror to mirror
     ```sh
    oc-mirror --config imageset-config.yaml docker://localhost:5000
    ```
### Additional Features
- Get information on your imageset using `describe`
    ```sh
    oc-mirror describe /path/to/archives
    ```

## Mirroring Process

During the create phase, a declarative configuration is referenced to download container images. Depending on the state of the workspace, the behavior of `create` will either package all downloaded images into an imageset or only the missing artifacts needed in the target environment will be packaged into an imageset.

> **Deep Dive:** The mirroring process assigns a UUID to the created workspace (either local or remote), which is used to track instances of metadata. Another assigned value is the sequence number. This value is assigned to each imageset to ensure the contents are publish in order. Both of these values are stored in the produced metadata file.

### Running `oc-mirror` For First Time
To create a new full imageset, use the following command with the target directory being a new, empty location and the configuration file authored referencing the config spec for the version of oc-mirror:

`oc-mirror --config imageset-config.yaml file://archives`


> **WARNING**: Depending on the configuration file used, this process may download multiple hundreds of gigabytes of data. This may take quite a while. Use the optional `log-level=debug` command line flag for more verbose output to track command execution.

**Note:** After `oc-mirror` has finished, an imageset named mirror_seq1_000000.tar will have been created and available in your specified directory. Use this file with `oc-mirror` to mirror the imageset to a disconnected registry:

`oc-mirror --from archives docker://localhost:5000`

- With a defined top-level namespace:  
`oc-mirror --from archives docker://localhost:5000/mynamespace`

### Running `oc-mirror` For Differential Updates

Once a full imageset has been created and published, differential imagesets that contain only updated images as per the configuration file can be generated with the same command as above:

`oc-mirror --config imageset-config.yaml file://archives`
## Glossary

`imageset` - Refers to the artifact or collection of artifacts produced by `oc-mirror`. 