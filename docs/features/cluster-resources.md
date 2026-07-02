# Cluster Resources Generation

## Overview

After mirroring images to a destination registry (via mirror-to-mirror or disk-to-mirror workflows), oc-mirror automatically generates Kubernetes manifests that configure an OpenShift cluster to use the mirrored content. These manifests are written to the `working-dir/cluster-resources/` directory.

The generated resources tell the cluster where to find mirrored images, how to access mirrored operator catalogs, and how to use the mirrored update graph for cluster upgrades.

## Output location

All cluster resource files are written to:

```
<workspace>/working-dir/cluster-resources/
```

Where `<workspace>` is:
- The `file://` destination path for mirror-to-disk workflows
- The `--workspace` path for mirror-to-mirror workflows
- The `--from` path for disk-to-mirror workflows

## Generated resources

### ImageDigestMirrorSet (IDMS)

**File:** `idms-oc-mirror.yaml`

An IDMS resource tells the cluster to pull digest-referenced images from the mirror registry instead of the original source. This is the primary mechanism for redirecting image pulls in a disconnected environment.

```yaml
apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: idms-oc-mirror-release-0
spec:
  imageDigestMirrors:
    - source: quay.io/openshift-release-dev/ocp-release
      mirrors:
        - registry.example.com/openshift-release-dev/ocp-release
```

Generated when any images referenced by digest are mirrored. Replaces the older ImageContentSourcePolicy (ICSP) resource from OpenShift 4.13 and earlier.

### ImageTagMirrorSet (ITMS)

**File:** `itms-oc-mirror.yaml`

An ITMS resource is similar to IDMS but handles images referenced by tag rather than digest.

```yaml
apiVersion: config.openshift.io/v1
kind: ImageTagMirrorSet
metadata:
  name: itms-oc-mirror-0
spec:
  imageTagMirrors:
    - source: registry.redhat.io/redhat/redhat-operator-index
      mirrors:
        - registry.example.com/redhat/redhat-operator-index
```

Generated when tag-referenced images are mirrored (e.g., operator catalog images).

### CatalogSource

**File:** `cs-<catalog-name>-<suffix>.yaml`

A CatalogSource resource registers a mirrored operator catalog with OLM (Operator Lifecycle Manager v0) so that operators from the catalog are available for installation on the cluster.

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: cs-redhat-operator-index-v4-18
  namespace: openshift-marketplace
spec:
  image: registry.example.com/redhat/redhat-operator-index:v4.18
  sourceType: grpc
```

Generated for each operator catalog mirrored. The CatalogSource name is derived from the catalog image path and tag/digest.

Custom CatalogSource templates can be specified in the ImageSetConfiguration using the `targetCatalogSourceTemplate` field on each operator entry.

### ClusterCatalog

**File:** `cc-<catalog-name>-<suffix>.yaml`

A ClusterCatalog resource registers a mirrored operator catalog with OLM v1 (the next-generation operator lifecycle manager).

```yaml
apiVersion: olm.operatorframework.io/v1
kind: ClusterCatalog
metadata:
  name: cc-redhat-operator-index-v4-18
spec:
  source:
    type: Image
    image:
      ref: registry.example.com/redhat/redhat-operator-index:v4.18
```

Generated alongside CatalogSource resources for each mirrored operator catalog.

### UpdateService

**File:** `updateService.yaml`

An UpdateService resource configures the OpenShift Update Service (OSUS) to use the mirrored graph data image for calculating available cluster upgrades.

```yaml
apiVersion: updateservice.operator.openshift.io/v1
kind: UpdateService
metadata:
  name: update-service-oc-mirror
  namespace: openshift-update-service
spec:
  replicas: 2
  releases: registry.example.com/openshift-release-dev/ocp-release
  graphDataImage: registry.example.com/openshift/graph-data@sha256:abc123...
```

Generated when release images are mirrored with `graph: true` in the ImageSetConfiguration. The graph data image contains the Cincinnati update graph that OSUS uses to determine available upgrade paths.

### Signature ConfigMap

**Directory:** `signatures/`

ConfigMap resources containing release image signatures, used by the cluster to verify the authenticity of mirrored release images.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: mirrored-release-signatures
  namespace: openshift-config-managed
  labels:
    release.openshift.io/verification-signatures: ""
binaryData:
  sha256-<digest>-<index>: <base64-encoded-signature>
```

Generated when release images with valid signatures are mirrored.

## Applying resources to a cluster

After mirroring, apply the generated resources to your OpenShift cluster:

```bash
oc apply -f <workspace>/working-dir/cluster-resources/
```

Or apply individual resources selectively:

```bash
oc apply -f <workspace>/working-dir/cluster-resources/idms-oc-mirror.yaml
oc apply -f <workspace>/working-dir/cluster-resources/itms-oc-mirror.yaml
oc apply -f <workspace>/working-dir/cluster-resources/cs-redhat-operator-index-v4-18.yaml
oc apply -f <workspace>/working-dir/cluster-resources/updateService.yaml
```

For signature ConfigMaps:

```bash
oc apply -f <workspace>/working-dir/cluster-resources/signatures/
```

**Note:** Applying IDMS or ITMS resources will trigger a rolling restart of nodes on the cluster as the machine config operator updates the container runtime configuration.

## Dry-run mode

Cluster resources are also generated in [dry-run](dry-run.md) mode for mirror-to-mirror and disk-to-mirror workflows, allowing you to preview the manifests that would be created. They are not generated for mirror-to-disk dry runs since the target registry is not known at that stage.

## Related documentation

- [Mirroring Workflows](mirroring-workflows.md) — How the three mirroring modes work
- [Enclave Support](enclave_support.md) — Setting up UpdateService (OSUS) in disconnected environments
- [Filtering](filtering.md) — Controlling which content is mirrored
