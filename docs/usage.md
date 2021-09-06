# Usage

## Overview

**Notice:** These commands are early alpha and may change significantly between application versions. 

oc-bundle has two main modes of operation: `create` and `publish`. 

## Prerequisites

### Authentication: 
oc-bundle currently retrieves registry credentials from `~/.docker/config.json`. Make sure that your [Red Hat OpenShift Pull Secret](https://console.redhat.com/openshift/install/aws/installer-provisioned) and any other needed registry credentials are populated in `~/.docker/config.json`

### Certificate Trust

oc-bundle currently references the host system for certificate trust information. For now, you must [add all certificates (trust chain) to be trusted to the System-Wide Trust Store](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/7/html/security_guide/sec-shared-system-certificates)


## Create  
During the create phase, a declarative configuration is referenced to download container images. Depending on the state of the workspace, the behavior of `create` will either package all downloaded images into an imageset or only the missing artifacts needed in the target environment will be packaged into an imageset. 

### Create Full
To create a new full imageset, use the following command with the target directory being a new, empty location and the configuration file authored referencing the config spec for the version of oc-bundle:

`oc-bundle create full --dir=<< empty directory >> --config=<< Path to config file >>`

**Note 1:** Depending on the configuration file used, this process may download multiple-hundreds of gigabytes of data. This may take quite a while. Use the optional `log-level=debug` command line flag for more verbose output to track command execution. 

**Note 2:** After `oc-bundle create full` has finished, an imageset named bundle_000000.tar will have been created and available in your current directory. Use this file with `oc-bundle publish` to mirror the imageset to a disconnected registry.

### Create Diff

Once a full imageset has been created and published, differential imagesets that contain only updated images as per the configuration file can be generated with the following command:

`oc-bundle create diff --dir=< empty directory >> --config=<< path to config file >>`

**Note 1:** Depending on the configuration file used and the periodicity between running `oc-bundle create diff`, this process may download multiple-hundreds of gigabytes of data, though differential updates should usually result in a significantly smaller imagesets.  

**Note 2:** After `oc-bundle create diff` has finished, an imageset named bundle_000000.tar will have been created and available in your current directory. This file must be used in sequence and with a publishing workspace that has a matching UUID.

## Publish

During the publishing phase, an imageset is checked against the state of a publishing workspace and then mirrored from file to a target registry server. Full imagesets must be used in conjunction with an empty workspace and differential imagesets must be published in sequence with a workspace that shares the same UUID as the incoming imageset.

To publish a full imageset use the following command:

`oc-bundle publish --dir=<< Empty Directory >> --archive=bundle_000000.tar --to-mirror=<< fqdn:port of target mirror >>`

To publish a differential imageset use the following command:

`oc-bundle publish --dir=<< previously used dir >> --archive=bundle_000000.tar --to-mirror=<< fqdn:port of target mirror >>`














