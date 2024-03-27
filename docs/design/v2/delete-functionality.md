# Image Delete Funaionality in V2


## Overview

In V1, pruning (or deletion of images) is set automatically, if a specific index (release, catalog) or additionalImage
is to be deleted, the current workflow would be to use the same "StorageConfig" setting and omit  the previously mirrored images, the pruning logic will find the "difference"
and prune the images accordingly.

This has its pros and cons, but due to the large amount of bugs been filed, we decided to change the behaviour in v2.

In v2 a new kind called "DeleteImageSetConfiguration" has been introduced, this is used to configure the deletion of images, to avoid using 
an ImageSetConfig, accidentally deleting wanted (deployed) images.

A new oc-mirror delete sub command is now available to execute the delete workflow with the DeleteImageSetConfiguration input.

This functionality is able to work in a complete "air-gapped" (disconnected) environment, providing that the images have been
previously mirrored (i.e the local cache and remote registry know about the images being deleted).

The delete functionality is split into 2 stages:

- The first stage is to create a delete yaml file using the --dry-run flag, this file is used to validate the images/blobs that will be deleted.
- The second stage is to use the delete yaml file (once validated) to actually perform a local cache and remote registry delete.


## Usage

Here is an example of the DeleteImageSetConfig:

```yaml
---
apiVersion: mirror.openshift.io/v1alpha2
kind: DeleteImageSetConfiguration
delete:
  platform:
    channels:
      - name: stable-4.13 
        minVersion: 4.13.3
        maxVersion: 4.13.3
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.12
      packages:
      - name: aws-load-balancer-operator
  additionalImages: 
    - name: registry.redhat.io/ubi8/ubi@sha256:bce7e9f69fb7d4533447232478fd825811c760288f87a35699f9c8f030f2c1a6
    - name: registry.redhat.io/ubi8/ubi-minimal@sha256:8bedbe742f140108897fb3532068e8316900d9814f399d676ac78b46e740e34e
```

Its immediatley obvious that "kind" and "mirror" fields have changed compared to the ImageSetConfig, this is to ensure
that all the relevant filtering and validation are done correctly, without having to update the internal code drastically.

The "delete" entry is the main entry, it contains the "platform", "operators" and "additionalImages" entries, these are used to filter the images
to intentionaly delete the manifests and blobs (in cache only).

### Command line examples

```bash
# stage 1
# -------

# create delete yaml file for delete phase using --dry-run
oc-mirror delete --config delete-image-set-config.yaml --source file://<previously-mirrored-work-folder> --v2 --dry-run

# use delete-id (usefull for comparing 2 versions of the delete yaml) 
oc-mirror delete --config delete-image-set-config-v4-15.yaml --source file://<previously-mirrored-work-folder> --v2 --dry-run --delete-id v4.15


# stage 2
# -------

# once the dry-run has executed succesfully use the created delete yaml to delete the images from the remote registry
# the default setting for the delete-yaml-file is <previously-mirrored-work-folder>/delete/delete-images.yaml
# delete remote registry only 
oc-mirror delete --config delete-image-set-config --source file://<previously-mirrored-work-folder> --destination docker://<remote-registry> --v2 --skip-cache-delete 

# delete remote registry only with yaml file specified
oc-mirror delete --config delete-image-set-config --delete-yaml-file <path-to-handcrafted-or-updated-delete.yaml> --source file://<previously-mirrored-work-folder> --destination docker://<remote-registry> --v2 --skip-cache-delete 

# delete remote registry and local cache 
oc-mirror delete --config delete-imagese-config --source file://<previously-mirrored-work-folder> --destination docker://<remote-registry> --v2  

```

### Description

The flags --source and --destination have intentionlly been introduced as to avoid any similarity with the current oc-mirror workflow,
ensuring no accidental deletion of images.

The --source flag must have the `file://` prefix and with the path to the previosly mirrored images, this flag is mandatory when using the --dry-run flag.

The --destination flag is the remote registry to delete the images from, this flag is optional. It is used in stage 2 and must have a `docker://` prefix

The --dry-run flag is used to create the delete yaml files in stage 1.
The file created can be found in the <previously-mirrored-work-folder>/delete/delete-images.yaml 

The --delete-id flag is used to create files in the delete folder with an id for version comparison and validation, in the form of <previously-mirrored-work-folder>/delete/delete-images-<delete-id>.yaml


Generated delete yaml, an actual example for the ubi8 image deletion:

```yaml
---
kind: DeleteImageSetConfiguration 
apiVersion: mirror.openshift.io/v1alpha2
items:
- imageReference: ubi8/ubi-minimal@sha256:8bedbe742f140108897fb3532068e8316900d9814f399d676ac78b46e740e34e
  imageName: registry.redhat.io/ubi8/ubi-minimal@sha256:8bedbe742f140108897fb3532068e8316900d9814f399d676ac78b46e740e34e
  relatedBlobs:
  - sha256:f0dc20fdb65a920a81ec9cd7bcbb294d875a4115c11a15e1daf442c80a54dc70
  - sha256:3599fcb6113c68e4b8e4a8b7a41e5df0f1527c53f0d3b4a513becc473fe0479d
  - sha256:8bedbe742f140108897fb3532068e8316900d9814f399d676ac78b46e740e34e
  - sha256:d917ca6754699f2f655c08fe820d0a4eb5d1ae4900d85ba7daebe5b8ee591be5
  - sha256:b5bcd41753d916f810874011f8b3549213ed697921afe2ba3ab526ba24f29286
  - sha256:8ee29dd270f33c66001e5fa10b72802df65634c8398ffb5ef6037271eaf6c829
  - sha256:a1e2f9104684775954779925a243fb4f97c77f1bda24b369408d4e820e175765
  - sha256:0688bc318d1720247e20e36d99aae20abd9955a8dc3afd7f0200266746a2a5fe
  - sha256:53e89ef9c7ad86030aeb60879f35d479d010c030f425f5a2a514d9e4511873ca
  - sha256:87e3ab05d9a4afadab5e2fe35fa8150a7a01c25ac51130a933b585d3dfa0f05c
  - sha256:28bcc8eb10d484228552ad05674411a96b4d5b6fa6c3517692e29f26b277683d
  - sha256:93b9f4797128c22f48bb7c7ec201dc251d40058fdb0ae7f5e4971b185daeed4f
  - sha256:e3649b5f99e0fcf52b3d19c3b201484e78f19873fb98a5b5a4b3a4d64c75ae78
```

**N.B.  A note about the remote registry deletion, only manifests are deleted (no blobs), it's out of scope for oc-mirror
to make an informed decision about deleting blobs on the remote-registry due to potential impact on dependencies etc.
The task of removing blobs for a remote registry will be left to the system administrator.**

### Troubleshooting and Recovery

The delete functionality is split into 2 stages (as mentioned in the overview), a typical workflow would be to use the --dry-run flag first, this will create the delete yaml file, this file can be used to validate the images/blobs that will be deleted.

We highly recommend making backup of each tar.gz file that gets created in the <work-directory> folder. You could also create a tar.gz using the --since flag (see the oc-mirror command help for more information).

Use the --delete-id flag to create various files in the delete folder (artifacts created by the delete functionality) for version comparison and validation.

For release image deletion it's recommended to be very specific with versions, i.e in making the minVersion and maxVersion entries the same.

For operators its recommended to also ensure specific versions, also keep in mind that if only one package in a catalog is deleted, take care in not deleting the actual catalog index from the remote repository or cache.

If by accident the local cache is deleted, untar all the relevant <work-directory>/mirror-xxxx-sequence.tar.gz files and copy the docker contents to the local cache directory
(default is ~/.oc-mirror/.cache), the local cache directory can be configured using the envar "OC_MIRROR_CACHE"

If the remote registry is also deleted by accident, re-run the oc-mirror command using the --from flag (disk to mirror mode, it will use the contents of all the relevant .tar.gz files), this will ensure your remote registry is back to the orifginal state.

### Conclusion

This is still tech preview so feedback is welcome.



