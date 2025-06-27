# Investigation about concurrency + progress bar log on oc-mirror

This blog summarizes the study around concurrently mirroring images and how to display the progress to the end user in the console. 

## Problems: 

1. Calling containers/image copy.Image in multiple go routines could lead to problems (critical).
2. The progress bar information from `containers/image` `copy.Image` become tangled when mirroring several images in parallel and this leads to confusion.


### Problem 1: Can we perform multiple mirrorings in parallel?

According to `containers/image` some registries [are not thread safe](https://github.com/containers/image/blob/dc519780d39f4abf2753a08b913c7edadffdf8ed/copy/copy.go#L254) (example: openshift image - internal registry and oci archive). 

This is the case for 2 transports, which **are not used by oc-mirror**:
* `oci-archive://` is not thread safe to [pull](https://github.com/containers/image/blob/main/oci/archive/oci_src.go#L131) (GET http method on SRC registry) and [push](https://github.com/containers/image/blob/dc519780d39f4abf2753a08b913c7edadffdf8ed/oci/archive/oci_dest.go#L97) (PUT http method on DEST registry) blobs. 
  * This format is not used in oc-mirror

* openshift image is not thread safe to [pull](https://github.com/containers/image/blob/dc519780d39f4abf2753a08b913c7edadffdf8ed/openshift/openshift_src.go#L86) (GET http method on SRC registry) and [push](https://github.com/containers/image/blob/dc519780d39f4abf2753a08b913c7edadffdf8ed/openshift/openshift_dest.go#L105) (PUT http method on DEST registry).
  * This does not mean that openshift rinternal registries are unsafe. Quite the contrary. 
  * This is in relation to `atomic://` which is deprecated for a very long time.
  * This simply means that mirroring to/from `atomic://` will not be thread safe.
  * This format is not used by oc-mirror

#### Scenario where problems could happen because of some registries are not thread safe:

Image 1 in the go routine 1 is pushing (PUT method) the blob *abc* to the registry (local cache or remote). It is still in progress (47% for example, this is very fast milliseconds or less).

Image 2 in the go routine 2 also contains the blob *abc* and it is pushing (PUT method) to the registry (local or remote) while the image 1 is still in progress (47%).

In registries which are not thread safe, one of them (Image 1 or Image 2) is going to fail. 

This could be the root cause of the current problems oc-mirror is facing with registries when pulling and pushing from/to registries.

#### Is [distribution/distribution](https://github.com/distribution/distribution/) the best choice for oc-mirror's internal cache?
A rapid [study](https://hackmd.io/Sxi9ezotRJmSLuwgaBgqFQ) of the existing registries showed that distribution/distribution is one of the only registries that :
* can be embeddable in a golang binary
* does not have too many dependencies
* is OCI and docker v2s2 compliant
* can use the filesystem as backing storage
* is robust, and can handle concurrency, to a certain limit

Studied registries: 
* [Spegel](https://github.com/spegel-org/spegel)
* [Quay](https://docs.redhat.com/en/documentation/red_hat_quay/3)
* [Harbor](https://github.com/goharbor/harbor)
* [Zot](https://zotregistry.dev/v2.1.0/)
* [Kraken](https://github.com/uber/kraken?tab=readme-ov-file)
* [Oras lightweight registry](https://oras.land/blog/lightweight-cloud-registry-oras)
* [oci-registry](https://github.com/mcronce/oci-registry)
#### Is [distribution/distribution](https://github.com/distribution/distribution/) thread safe

In the case of oc-mirror, the storage driver used is: filesystem. so POSIX
For a file system to be compatible with POSIX, it must:
* Implement strong consistency. For example, if a write happened before a read, the read must return the data written.
* Have atomic writes, where a read either returns all data written by a concurrent write or none of the data but is not an incomplete write.
* Implement certain operations, like random reads, writes, truncate, or fsync.
* Control access to files using permissions (see here) and implement calls like chmod, chown, and so on to modify them.

POSIX doesn't say much about concurrent writes... so this means the caller needs to provide some mechanism to prevent concurrent writes.

In [distribution/distribution](https://github.com/distribution/distribution/), things like semaphores seem to be used around writing blobs.
Each time a blob is being uploaded, the digest is written to a map (protected by a mutex). A second connection will not be able to add the digest again to the map.

Furthermore blobWriter does:
* receive the content to a temporary location
* Commit
  * verify that the digest provided in the request matches the one calculated based on full blob content written
  * and only then move the blob to its final location
  * Commit... putting the digest in the metadata of the registry

So that being said, it does sound less bad than we think.
Afterall, HA registries like Docker and Harbor are based on this!
Although not sure they use the same driver as us 

### Solution Problem 1:

From the above, mirroring images can be done concurrently.
Currently `containers/image` uses option [MaxParallelDownloads](https://github.com/aguidirh/image/blob/main/copy/copy.go#L120) to adjust the number of go routines used to copy layers of a certain image.

By default, `containers/image` sets the default value of this option to 6.

In oc-mirror, we opted to create a `--max-parallel-downloads` flag, in order to harmonize flags used by RedHat created CLIs.

From the value of this flag, oc-mirror determines:
* the maximum number of layers that can be copied concurrently : This is `containers/image`option `MaxParallelDownload`.
  * The maximum number of layers copied concurrently is a fixed value : 6 - as in `containers/image`
  * The maximum number of images copied simultaneously. This is known as the batch size.

The default value for `--max-parallel-downloads` flag in oc-mirror is 48. This means:
* up to 6 layers are copied in parallel for a given image
* at each point in time, up to 8 images are being copied in parallel.

A maximum value of 64 is set for `--max-parallel-downloads`, which means that the limit is:
* up to 6 layers are copied in parallel for a given image
* at each point in time, up to 10 images are being copied in parallel.


### Problem 2

Calling copy.Image in parallel makes console output showing the layer copy progress very confusing. It happens because since there is no sequence between the images being downloaded, it is not possible to determine for which image a blob being downloaded belongs to.

### Solution Problem 2

oc-mirror masks the progress bars on layers provided by `containers/image`.

Instead, it displays a spinner for each image getting mirrored.

To give the user a general idea of the overall progress on the complete image set, oc-mirror also displays with each spinner, the image index and the total number of images.


## Performance test

Mirror To Mirror was performed using the following ImageSetConfiguration:

```
kind: ImageSetConfiguration
apiVersion: mirror.openshift.io/v1alpha2
mirror:
  platform:
    channels:
    - name: stable-4.15
      minVersion: 4.15.17
      maxVersion: 4.15.17
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
  - name: hello-world:latest
```
After each test, cache, working-dir and mirror registry storage (~/.local/share/containers/storage) are cleared.

### Results:

|version|elapsed time|
|---|---|
|v1|40m14s|
|v2 4.16|47m40s|
|v2 current|39m38s|
|v2 current 10//layers*8//images|28m36s|


