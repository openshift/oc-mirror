# Investigation about concurrency + progress bar log on oc-mirror

## Problems: 

1. Calling containers/image copy.Image in multiple go routines could lead to problems (critical).
2. Progress bar when calling containers/image copy.Image in multiple go routines is confusing.

## Why is it a problem? 

### Problem 1:

According to containers/image some registries [are not thread safe](https://github.com/containers/image/blob/dc519780d39f4abf2753a08b913c7edadffdf8ed/copy/copy.go#L254) (example: openshift image - internal registry and oci archive). 

* oci archive is not thread safe to [pull](https://github.com/containers/image/blob/main/oci/archive/oci_src.go#L131) (GET http method on SRC registry) and [push](https://github.com/containers/image/blob/dc519780d39f4abf2753a08b913c7edadffdf8ed/oci/archive/oci_dest.go#L97) (PUT http method on DEST registry) blobs. 

* openshift image is not thread safe to [pull](https://github.com/containers/image/blob/dc519780d39f4abf2753a08b913c7edadffdf8ed/openshift/openshift_src.go#L86) (GET http method on SRC registry) and [push](https://github.com/containers/image/blob/dc519780d39f4abf2753a08b913c7edadffdf8ed/openshift/openshift_dest.go#L105) (PUT http method on DEST registry).

#### Scenario where problems could happen because of some registries are not thread safe:

Image 1 in the go routine 1 is pushing (PUT method) the blob *abc* to the registry (local cache or remote). It is still in progress (47% for example, this is very fast milliseconds or less).

Image 2 in the go routine 2 also contains the blob *abc* and it is pushing (PUT method) to the registry (local or remote) while the image 1 is still in progress (47%).

In registries which are not thread safe, one of them (Image 1 or Image 2) is going to fail. 

This could be the root cause of the current problems oc-mirror is facing with registries when pulling and pushing from/to registries.

### Solution Problem 1:

Currently containers/image is handling the scenario for registries which are not thread safe. It identifies the kind of registry and disable the go routines in case it is not thread safe. For registires which are thread safe it is possible to increase the number of go routines by using the option [MaxParallelDownloads](https://github.com/aguidirh/image/blob/main/copy/copy.go#L120)

In order to let containers/image handle the scenario, oc-mirror needs to call copy.Image sequentially instead of in multiple go routine like it is implemented today.

Advantages:
* Safer since no conflicts between blobs when processing src and dest registries (GET and PUT respectively)
* Reuse of blobs already present in the registry in subsequent calls of copy.Image

Disadvantages:
* If no blobs are shared between images, the mirroring process could be slower.

### Problem 2

Calling copy.Image in parallel makes the progress bar very confusing. It happens because since there is no sequence between the images being downloaded, it is not possible to determine for which image a blob being downloaded belongs to.

### Solution Problem 2

Calling containers/image copy.Image sequentially allow oc-mirror to show the progress bar and follow the progress of each image being downloaded.


## Performance test

Mirror To Disk was performed using the following ImageSetConfiguration:

```
kind: ImageSetConfiguration
apiVersion: mirror.openshift.io/v1alpha2
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

### Results:

Current implementation with multiple go routines: 16m22.383589103s

Solution calling containers/image copy.Image sequentially without go routines: 24m15.248946558s

Even the current solution being faster, there is no guarantee all the blobs were pushed since conflicts may have ocurred during the process as already explained above.


