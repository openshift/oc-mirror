

# oc-mirror Design Overview

## Summary 
oc-mirror assists with lifecycle management of mirror registries that support OpenShift environments by performing differential update operations and synchronizations.   
  
## Features
1. Differential Synchronization of OpenShift Release Images
2. Differential Synchronization of Operator images
3. Synchronization of OpenShift cincinnati graph data image (planned)
4. Synchronization of OpenShift Release images by version or channel latest
5. Synchronization of Operator Images by full catalog and full catalog at latest operator versions. Also, allows for synchronization of individual operators by version to channel(s') HEAD.
6. Synchronization of helm charts
7. Synchronization of additional images
8. Remote state backends
9. Base image filtering (planned)
10. (multi) Cluster resource management

## Design Considerations
1. Deterministic -  Each application phase’s results are the same regardless of where the application is executed from within each phase’s environment. This allows for oc-mirror to underpin a website, run in CI, reconcile as an operator, or run from the CLI as a standalone binary or as an oc plugin.
2. Stateful - Differential operations require tracking downloaded versions and calculating upgrade paths based on previous application runs. State must persist between execution environments.
3. One-way synchronization - Some internet-disconnected environments are considered confidential and cannot tolerate data spillage from the disconnected environment to any internet connected networks. oc-mirror only synchronizes differential state from internet-connected environments to internet-disconnected environments. oc-mirror assumes that no data can traverse from an internet-disconnected environment to an internet-connected environment at any time. 
4. User Workloads -  oc-mirror synchronizes helm charts and arbitrary images to streamline the importation of user workloads into their OpenShift deployments. 
5. Internet Connected Experience - Operator Lifecycle Manager, OpenShift Update Service, Advanced Cluster Management, and oc-mirror; in concert, can facilitate user experience parity with that of internet-connected environments. This ux is achieved by updating cluster resources during the publishing phase of oc-mirror(planned). 
6. Artifact Size - oc-mirror attempts to reduce the size of artifacts passed from the internet-connected environment to the internet-disconnected environment. These efficiencies are addressed at two levels: Differential version updates and Differential image layer updates. These artifacts can be further subdivided into user-defined file sizes. During each phase of operation, every effort has been made to reduce duplication of content on disk while contents traverse between image registries and imagesets. Due to image validation requirements, network utilization remains the most inefficient aspect of oc-mirror.
7. Wide Applicability - oc-mirror provides a means to synchronize container image content for limited connectivity deployments of containerized edge, IoT, and AI/ML offerings. 

## Key Components
1. Metadata - Complete record of all content synchronization
2. UUID - Ensures synchronized content integrity
3. Sequence - Ensures differential content integrity
4. Workspace - Environment used to create and publish content
5. Backends - Location of metadata
6. Imageset - Medium for transferring content and metadata between environments.

## Operation
oc-mirror has two main phases of operation:

1. Create 
2. Publish

And two modes of operation:

1. Full
2. Diff

During the “create full”  phase, the following operations are performed:

1. Read declarative configuration file
2. Create new workspace
3. Generate new UUID
4. Assign sequence number 1
5. Download/Archive content
6. Write metadata to imageset
7. Write metadata to backend

During the “publish full” phase:

1. Metadata is read from imageset
2. UUID/Sequence number checking with backend metadata
3. Upload content to mirror
4. Output k8s manifests
4. Optionally update (multi) cluster ICSPs / Catalog Source / OpenShift Update service
5. Update backend metadata

During the “create diff” phase for differential updates:

1. Read declarative configuration file
2. Check UUID of workspace
3. Assign sequence number n+1
4. Differentially download / archive content (download diff and throw away previously synchronized image layers)
5. Write metadata to imageset
6. Write metadata to backend

During the “publish diff” phase for differential updates:

1. Metadata is read from imageset
2. UUID/Sequence number checking with backend metadata (must be next in sequence)
3. Merge differential operator upgrade graph into the existing catalog index image
4. Upload content to mirror (reconstitute images from new layers and previously uploaded layers) 
5. Output cluster ICSPs / Catalog Source / Cincinnati
6. Optionally update (multi) cluster ICSPs / Catalog Source / Cincinnati 
7. Update backend metadata
