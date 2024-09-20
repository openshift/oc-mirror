# OCPSTRATS-1515 - Reflecting the mirrored operators only in catalogs mirrored by oc-mirror

**This document is a work in progress and will be updated along with the decisions and implementations that will come in the future.**

## Table of contents
- [OCPSTRATS-1515 - Reflecting the mirrored operators only in catalogs mirrored by oc-mirror](#ocpstrats-1515---reflecting-the-mirrored-operators-only-in-catalogs-mirrored-by-oc-mirror)
  - [Table of contents](#table-of-contents)
  - [Background](#background)
  - [Feature request](#feature-request)
  - [Selected solution for OLM-v0 catalogs](#selected-solution-for-olm-v0-catalogs)
  - [Regarding OCPSTRAT-1417](#regarding-ocpstrat-1417)
    - [Description](#description)
    - [TLDR; complexity level estimation](#tldr-complexity-level-estimation)
    - [Generating the FBC](#generating-the-fbc)
      - [Requirements](#requirements)
      - [Solution](#solution)
        - [Pros](#pros)
        - [Cons](#cons)
        - [Complexity](#complexity)
    - [Preparing catalog files](#preparing-catalog-files)
      - [Requirements](#requirements-1)
      - [Solution](#solution-1)
      - [Complexity](#complexity-1)
    - [Building the container image](#building-the-container-image)
      - [Requirements](#requirements-2)
      - [Implementation](#implementation)
      - [Complexity level estimation](#complexity-level-estimation)
  - [Risks](#risks)


## Background

In oc-mirror v2, when mirroring catalogs, catalog images are not rebuilt. 
* The filtered declarative config isn't recreated based on the imagesetconfig filter
* The catalog cache isn't regenerated
* The catalog image isn't rebuilt based on the above 2 elements
* Instead, the original catalog image is pushed as is to the mirror registry. Its declarative config will show all operators, and for each operator all channels and all bundles.
* This behavior is causing some inconvenience to our users, especially 
  * [OCPBUGS-35386](https://issues.redhat.com/browse/OCPBUGS-35386): Since the catalogs include the unfiltered FBC, deploying the catalogsource to cluster will result in the creation of install plans for all operators founds in the catalog. These install plans are going to fail for operator bundles that haven't been mirrored.
  * In the web console of openshift, the catalog appears to contain more operators than what was really filtered.


## Feature request

oc-mirror customers would like the mirrored catalog to reflect the reality of the operators that are really available in the mirrored registy.

This way, on air-gapped clusters, install plans will be generated only for bundle versions that are in fact available to the cluster. 

This behavior SHOULD NOT interfere with the ability to verify the catalog (or contents) signature verification, as requested in [OCPSTRAT-1417](https://issues.redhat.com/browse/OCPSTRAT-1417).

Possible solutions were [discussed with Operator Framework team](https://drive.google.com/file/d/1-O1MX-tnGntrl7EMfdjekSIHsUm8TOD3/view?usp=drive_web), and the gist of the discussion shows that:
* **For OLM-v1 catalogs**, several solutions for filtering out catalogs while preserving image's signature and SBOM are possible. A separate feature request should be created. Among the solutions mentioned:
  * Adding a patch with filtering metadata as a [OCI referrer](https://github.com/oras-project/artifacts-spec/blob/main/manifest-referrers-api.md)
  * Using extra fields in the cluster catalog custom resource
* **For OLM-v0 catalogs**, no perfect solution exists that ensures signature validity at the same time. Among the solutions mentioned:
  * no filtering: which keeps the image intact, and inline with its signature. On the downside, clusters will still suffer from [OCPBUGS-35386](https://issues.redhat.com/browse/OCPBUGS-35386).
  * a release concept attached to the catalog image, that can be used by OLM, the webconsole, etc to patch the original FBC. CatalogSource can have extra stanza, `patches` for example. But SQLite catalogs cannot be patched. Additionally, extra effort on a soon to be deprecated version is going to be a waste.
  * mirroring by minVersion only: Contrary to v1, oc-mirror v2 would not allow filtering by maxVersion. As a result, more operator bundles will be mirrored, up to each channel head. The catalog image is also kept intact in this solution, so inline with its signature. The clusters won't experience [OCPBUGS-35386](https://issues.redhat.com/browse/OCPBUGS-35386) as the channel head bundle is always available. The downside of this solution is that oc-mirror customers might choose not to migrate to oc-mirror v2 because it lacks the filtering by maxVersion filtering.
  * filtering by minVersion and maxVersion: Preserving the oc-mirror v1 functionality, and rebuilding the catalog image from the filtered FBC. The downside here is that the signature verification will fail, and that the SBOM will not correspond to the filtered catalog image. 

## Selected solution for OLM-v0 catalogs

This document will only focus on the feature design for OLM-v0 catalogs. Reflecting the mirrored operators in OLM-v1 catalogs shall be handled in a different feature request, at a later time.

From the above solutions for OLM-v0 catalogs, the decision was made to rebuild catalog images inserting the FBC filtered by minVersion (and maxVersion) in the image. 

**This solution is TEMPORARY.** It shall be accompanied by proper documentation explaining that filtered catalogs will not benefit from signature verification, and that this problem will be **fully solved by migrating to OLM-v1 catalogs.**

## Regarding [OCPSTRAT-1417](https://issues.redhat.com/browse/OCPSTRAT-1417)

For the switch from OLM v0 to OLM v1, oc-mirror would need to switch from rebuilding catalogs and generating CatalogSource objects (compatible with rebuilt catalogs, without signature verification) to creating catalog patches (filtering metadata) and ClusterCatalog objects (compatible with OLM-v1, supporting signature verification). In this manner, a user cannot by mistake use a rebuilt catalog on a cluster with OLM-v1, nor a OLM-v1 compatible catalog on a cluster with OLM-v0. 

A signature policy needs to be generated by oc-mirror along with catalogSources in order to instruct the cluster that it should not verify signatures for OLM-v0 catalog images: Being rebuilt images, the catalog wouldn't be usable on the cluster if the signature verification is enforced.

:warning: **Future of `SelectedBundles`: OLM-v1 API would provide a much better way to pin down a bundle version (subscription API change). `SelectedBundles` should  become an invalid property when mirroring for OLM-v1**

### Description

Building a filtered catalog entails the following tasks:
1. **Generate a catalog FBC (File based Catalog)**, that contains the filtered operators, and their respective packages, channels and bundles. 
2. **Prepare the binaries and files for the filtered catalog**, which include the opm binary and eventually catalog cache that is compatible with FBC AND the opm version chosen.
3. **Building the container image**, based on the above.

In the following sections, we will attempt to analyze each task, describe the solution chosen and give an estimation of the level of complexity for the task. 

### TLDR; complexity level estimation

| Task | Complexity | 
|---|---|
| Generate catalog FBC | medium | 
| Prepare catalog files | medium |
| Build catalog image | medium | 

### Generating the FBC

#### Requirements
From the OLM perspective, a valid FBC requires: 
* A default channel that is included in the FBC
* No dangling versions, ie no versions that cannot reach the channel head
* A single channel head
  
#### Solution
Based on existing [POC](https://github.com/operator-framework/operator-registry/pull/1231), a new `CatalogFilter` implementation will be created in order to respect oc-mirror's filtering specificities: 
* When no filtering is set, only the head of the channel shall be returned
* when filtering config is empty, oc-mirror expects to have the head of each operator
* when filtering by operator name only, oc-mirror expects the channel head alone.  
* `full` is used in oc-mirror in order to specify that all the channel contents (not only head) needs to be rendered. 
* Filtering directly on minVersion and maxVersion (without specifying a channel) is recognized as valid filtering in oc-mirror. 
* Filtering by `SelectedBundles` is recognized as valid filtering in oc-mirror, but only for v0 catalogs. 

##### Pros
* Done by OLM, this POC closest probably to what the OLM on cluster would expect from an operator's upgrade path. 
* It's a step towards the introspection tool
* A filtering that relies on the FBC API (`replace`, `skip` and NOT `skipRange`) , instead of the current oc-mirror v2 implementation which simply relies on semver, and was originally designed as a temporary filtering.

##### Cons
* The POC implementation is not inline with what oc-mirror expects. A new implementation is necessary
* The current v2 filtering would need to be re-written in order to rely on this catalogFilter 

##### Complexity

In operator-registry [fork](https://github.com/sherine-k/operator-registry/tree/filter-fbc) and the following [demo](https://github.com/sherine-k/catalog-filter), we can show that starting from the PR, it was easy to create a new `CatalogFilter` implementation that respects oc-mirror's requirements for filtering. 

** This implementation passes 11/19 filtering use cases **. This implementation effort was timeboxed to 2days.

On these grounds, **complexity is estimated to medium**, with the following identified subtasks
* :white_check_mark: case of no filter at all should return heads of all operators
* :white_check_mark: case of filtering by operator name should return heads of those operators
* :white_check_mark: when `full` is used all channel contents are returned
* :white_square_button: case of filtering by min and/or max without any specified channels
* :white_square_button: case of filtering by selectedBundles
* :white_square_button: Creating an independant repository: [PR#1231](https://github.com/operator-framework/operator-registry/pull/1231) won't merge into opm. Filtering needs to be a separate tool. This is because, long term, the opm binary will only be useful for v0 operations.


### Preparing catalog files

#### Requirements
A catalog image is:
* based (FROM statement) on an opm image
* contains the FBC catalog, under the /configs folder
* contains the cache folder corresponding to the FBC catalog
* proper labels and CMD are needed in order to link everything together properly

#### Solution
From the discussion we had, it appears that **the ultimate solution** for this is to :
* update the FBC within the catalog image
* Not care about the cache. This is what we already do in oc-mirror v1 ([OCPBUGS-35386](https://issues.redhat.com/browse/OCPBUGS-35386)), updating the image's CMD to remove `--cache-dir` as in oc-mirror v1
* TO BE CONFIRMED for 4.18: Use the [.spec.grpcPodConfig.extractContent](https://docs.openshift.com/container-platform/4.15/rest_api/operatorhub_apis/catalogsource-operators-coreos-com-v1alpha1.html#spec-grpcpodconfig-extractcontent) api in the catalogSource in order to ignore the opm binary within the image (>4.15)
  * Alternative: update the image's CMD to remove `--cache-dir` as in oc-mirror v1.

This solution is valid for the following reasons:
* OPM cache generation is loading the FBC using pogrep (proto-buf) instead of JSON
* Cache generation is done package by package instead of loading the whole FBC
* Introduction of `olm.csv.metadata` in replacement for the fat `olm.bundle.object`. ART pipelines can start to be modified to start replacing

:warning: **This solution is to be validated for 4.14 clusters and catalogs. The `olm.csv.metadata` and `.spec.grpcPodConfig.extractContent` are only available starting 4.15**

PS: In the case where the cache generation has to be implemented, oc-mirror should adopt the same strategy as IIB:
* build the cache outside of the image (container build with a volume mount to be able to collect the cache)
* Use the same opm binary (in other words the catalog image as the source) to generate the cache. In this way the opm binary used to serve the FBC on cluster is the same one that generated the cache. 

Something like :
```
# `.RefExact` is the mirrored catalog
FROM {{ .RefExact }}
USER root
RUN rm -fr /configs
COPY ./index /configs
# In case we want to rebuild the cache
USER 1001
RUN rm -fr /tmp/cache/*
RUN /bin/opm serve /configs --cache-only --cache-dir=/tmp/cache
```

#### Complexity

Once the uncertainty about the implementation details taken away, this task should be a **small sized complexity task**.

### Building the container image

From the Dockerfile obtained in the previous task, we need to build the container image. 

#### Requirements 
* should be able to build images for catalogs that are registry based or file based . This should not be a problem: as stated in the previous section, we should not build the filtered catalog using FROM the catalog referenced in the imageSetConfig
* should build multi-arch images
* should be able to build images in enclave environment. ie. respect auth, tls, registries.conf and proxy as stated in flags and env vars

#### Implementation

Several Go modules provide a way to build containers in go:

| Solution | Description |
|---|---|
| Shell-out to podman | Creates an external dependency on podman.<br> Command can be different according to platform and OS.<br>Cannot build when on disk oci catalogs are used.<br> [POC](https://github.com/openshift/oc-mirror/commit/2db93ca48cd40daf06257650555b7f1cfd301417)|
| Buildah | Relies on `containers/image` suite.<br> Makes oc-mirror harder to debug.<br> Cannot build when on disk oci catalogs are used.<br> [POC](https://github.com/openshift/oc-mirror/commit/88472f2543d999c0e63bad161e65f91f77e512ab)
| ORAS |No POC|
| go-containerregistry| No POC. <br> Not compatible with registries.conf|

#### Complexity level estimation

Without POCs on the other modules, it is hard to estimate exactly the complexity of this issue.

## Risks

The main risk of rebuilding the catalog is that the signature, SBOM and other attestations will not match the new rebuilt catalog. 
This may lead to customers mistaking filtered catalogs with untrusted catalogs. 

