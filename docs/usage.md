# Usage

- [Usage](#usage)
  - [Overview](#overview)
  - [Prerequisites](#prerequisites)
    - [Authentication:](#authentication)
    - [Certificate Trust](#certificate-trust)
    - [Content Discovery](#content-discovery)
      - [Updates](#updates)
      - [Releases](#releases)
      - [Operators](#operators)
    - [Mirroring](#mirroring)
      - [Fully Disconnected](#fully-disconnected)
      - [Partially Disconnected](#partially-disconnected)
  - [Additional Features](#additional-features)
  - [Mirror to Disk](#mirror-to-disk)
    - [Running Mirror For First Time](#running-mirror-for-first-time)
    - [Running Mirror For Differential Updates](#running-mirror-for-differential-updates)

## Overview

**Notice:** These commands are early alpha and may change significantly between application versions. 

## Prerequisites

### Authentication: 
oc-mirror currently retrieves registry credentials from `~/.docker/config.json`. Make sure that your [Red Hat OpenShift Pull Secret](https://console.redhat.com/openshift/install/pull-secret) and any other needed registry credentials are populated in `~/.docker/config.json`

### Certificate Trust

oc-mirror currently references the host system for certificate trust information. For now, you must [add all certificates (trust chain) to be trusted to the System-Wide Trust Store](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/7/html/security_guide/sec-shared-system-certificates)


### Content Discovery

oc-mirror provides a way to discover release and operator content to allow users to successfully create
an imageset config with the data they need. The list update `list updates` command will differentiate
between past runs and the provided configuration to show what new versions exists.
#### Updates

- List updates since the last `oc-mirror` run
  ```sh
  ./bin/oc-mirror list updates --config imageset-config.yaml --dir test-create
  ```
** Note: ** You must have existing metadata in your workspace or registry (if using)to use `list updates`
#### Releases
1. List all available release payloads for a version of OpenShift (defaults to stable)
   ```sh
   ./bin/oc-mirror list releases --version=4.9
   ```
2. List all available channels to query for a version of OpenShift
   ```sh
   ./bin/oc-mirror list releases --channels --version=4.8
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

#### Fully Disconnected
- Create then publish to your mirror registry:
    ```sh
    ./bin/oc-mirror --config imageset-config.yaml --dir test-create file://archives
    ./bin/oc-mirror --from /path/to/archives --dir test-publish docker://reg.mirror.com
    ```
#### Partially Disconnected
- Publish mirror to mirror
     ```sh
    ./bin/oc-mirror --config imageset-config.yaml --dir test docker://localhost:5000
    ```
## Additional Features
- Get information on your imageset using `describe`
    ```sh
    ./bin/oc-mirror describe /path/to/archives
    ```

## Mirror to Disk 
During the create phase, a declarative configuration is referenced to download container images. Depending on the state of the workspace, the behavior of `create` will either package all downloaded images into an imageset or only the missing artifacts needed in the target environment will be packaged into an imageset. 

### Running Mirror For First Time
To create a new full imageset, use the following command with the target directory being a new, empty location and the configuration file authored referencing the config spec for the version of oc-mirror:

`./bin/oc-mirror --config imageset-config.yaml --dir test-create file://archives`

**Note 1:** Depending on the configuration file used, this process may download multiple-hundreds of gigabytes of data. This may take quite a while. Use the optional `log-level=debug` command line flag for more verbose output to track command execution. 

**Note 2:** After `oc-mirror` has finished, an imageset named mirror_seq1_000000.tar will have been created and available in your specified directory. Use this file with `oc-mirror` to mirror the imageset to a disconnected registry:

`./bin/oc-mirror --from archives --dir test-publish docker://localhost:5000`

### Running Mirror For Differential Updates

Once a full imageset has been created and published, differential imagesets that contain only updated images as per the configuration file can be generated with the same command as above:

`./bin/oc-mirror --config imageset-config.yaml --dir test-create file://archives`

**Note 1:** Depending on the configuration file used and the periodicity between running `oc-mirror`, this process may download multiple-hundreds of gigabytes of data, though differential updates should usually result in a significantly smaller imagesets.  

**Note 2:** The `--dir` value must be the same for all create runs if you use a local backend (the default option) to detect the metadata.













