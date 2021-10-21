# Overview

## Summary

oc-mirror is an oc plugin for lifecycle management of internet disconnected openshift environments. oc-mirror generates and publishes container image aggregation files referred to as imagesets. Imagesets combine container image content with robust intelligence for environment synchronization.

## Capabilities

1. Single command for downloading   
    - Red Hat OpenShift Release Images  
    - Kubernetes Operator Catalogs  
    - Arbitrary container images to support a target environment  
2. User configurable image blocking (Planned)  
    - Can block downloads of images based on user defined “unauthorized” base images  
3. User defined imageset file size  
4. Supports remote backends in internet connected and disconnected environments  
5. Single command for publishing to disconnected environments.  
    - Mirror from disk to disconnected registry  
    - Mirror from registry to disconnected registry (Coming Soon)  
    - “Installs” updates into target clusters (Coming Soon)  
6. Simple declarative configuration  
    - Yaml formatted config spec for declaring download/mirroring parameters  
7. One way Internet connected -> Internet Disconnected synchronization to facilitate differential updates  
8. Support for multi-cluster Internet disconnected environments  
    - Cluster version/Operator catalog version reconciliation  
    - Command line flag automatically installs ImageContentSourcePolicies and CatalogSources into a target cluster

## Differential Operation

 During differential imageset creation, if a shared image layer was included in a previously generated imageset, that layer is not added to subsequent imagesets. During the publishing phase, if an image layer is needed during mirroring of new container images, but not included in the incoming imageset, the needed layer is individually downloaded from the publishing target mirror, added to the reformed registry v2 image on disk, validated, and then the entire container image is uploaded to the target registry. 
 
 oc-mirror uses strict uuid/sequence tracking to ensure availability of imageset artifacts between differential imageset phases. When a new full imageset is created, a new UUID is generated for that workspace and imageset. A full imageset can only be published with an empty workspace. After the first full imageset is published with a new workspace, that workspace can only publish imagesets with matching UUIDs that are next in sequence. This helps ensure layers needed during publishing will be present during publishing of differentially generated imagesets.




