# oc-mirror Disconnected E2E CI Investigation

## Goal

The oc-mirror team wants to run OCP end-to-end tests in a simulated disconnected scenario. The flow is:

1. Mirror the Red Hat registry to an offline registry using oc-mirror.
2. Install an OpenShift cluster that uses only that offline registry (no internet access).
3. Run the standard OCP e2e test suite against the cluster.

This document captures what step-registry components already exist and what needs to be created.

---

## What already exists

The repo has a well-established disconnected AWS e2e workflow:
**`openshift-e2e-aws-disconnected`**
(`ci-operator/step-registry/openshift/e2e/aws/disconnected/openshift-e2e-aws-disconnected-workflow.yaml`)

Its `pre` chain (`ipi-aws-pre-disconnected`) does:

1. **`aws-provision-vpc-disconnected`** – provisions an isolated VPC with private subnets.
2. **`aws-provision-bastionhost`** – creates a bastion host on the public subnet running a mirror registry and egress proxy.
3. **`mirror-images-payload`** (optionally via `mirror-images-by-oc-mirror`) – mirrors the OCP release payload images to the bastion registry using oc-mirror v2.
4. **`ipi-conf-mirror`** / **`ipi-conf-aws`** – patches `install-config.yaml` with mirror registry settings.
5. **`ipi-install-install-aws`** – installs the cluster.

Then `test: openshift-e2e-test` runs the standard e2e suite.

There is also a general-purpose **`mirror-images-by-oc-mirror`** step/chain
(`ci-operator/step-registry/mirror-images/by-oc-mirror/`) that mirrors a release payload
using oc-mirror v2 directly to the bastion registry and produces IDMS/ITMS files for the installer.

---

## The critical gap

The existing disconnected workflow simulates **proxy-based disconnected**, not a **true air-gap**:

- The bastion host **has internet access** (it sits on the public subnet).
- Cluster nodes reach the outside world only through the bastion's egress proxy and mirror registry.
- Images are mirrored on-the-fly at install time, not pre-mirrored to an offline store first.

The oc-mirror team needs to test their product's core use case:
**mirror → offline store → disconnect → install → test**.

---

## Missing steps

### Step 1: Mirror content to disk archive (`file://`)

oc-mirror's primary air-gap workflow uses:
```
oc-mirror --config imageset.yaml file://<path>
```
to write a portable disk archive first.

No reusable step-registry component exists for this. The existing `mirror-images-by-oc-mirror`
step skips the intermediate phase and goes directly `docker://source → docker://target`.

**What is needed**: A step that runs oc-mirror with a configurable `ImageSetConfiguration`
(platform release + optionally operator catalogs) and writes the result to a disk archive
(`file://`) on the bastion or in `SHARED_DIR`.

Suggested name: `mirror-images-by-oc-mirror-to-disk`

---

### Step 2: Push disk archive to the offline registry

The second half of oc-mirror's workflow is:
```
oc-mirror --from file://<path> docker://<offline-registry>
```

No reusable step-registry component exists for this phase either. The
`hypershift/kubevirt/create/disconnected/workarounds` step script does this inline on a
baremetal host, but it is not a reusable step-registry component.

**What is needed**: A step that takes the disk archive produced in Step 1 and pushes it to
the target offline registry, then writes the resulting IDMS/ITMS files to `SHARED_DIR` for
the installer to consume.

Suggested name: `mirror-images-by-oc-mirror-from-disk`

---

### Step 3: Network isolation after mirroring

Currently the bastion can always reach the internet. For a true air-gap simulation, the
network route to the outside must be cut **after** the mirror operation completes and
**before** the cluster install begins.

**What is needed**: A step that enforces network isolation (e.g., removing the internet
gateway route, restricting the NAT gateway, or applying AWS security group rules) after
mirroring is complete.

Suggested name: `aws-provision-vpc-disconnect-internet`

---

### Step 4: End-to-end workflow

No workflow exists that wires the above together. The existing `openshift-e2e-aws-disconnected`
workflow could serve as a template but would need to replace the mirror chain with the
two-phase disk-archive approach and add the isolation step.

**What is needed**: A new workflow or chain that sequences:
1. Provision VPC + bastion with mirror registry
2. Mirror content to disk archive
3. Push disk archive to offline registry
4. Isolate network (cut internet access)
5. Install cluster from offline registry
6. Run OCP e2e tests

Suggested name: `openshift-e2e-aws-oc-mirror-disconnected`

---

## Summary

| Step | Exists? | Component |
|---|---|---|
| Provision isolated VPC + bastion with registry | Yes | `aws-provision-vpc-disconnected`, `aws-provision-bastionhost` |
| Mirror release payload (direct, no disk) | Yes | `mirror-images-by-oc-mirror` step/chain |
| Mirror content to disk archive (`file://`) | **No** | New: `mirror-images-by-oc-mirror-to-disk` |
| Push disk archive to offline registry | **No** | New: `mirror-images-by-oc-mirror-from-disk` |
| Enforce network isolation after mirroring | **No** | New: `aws-provision-vpc-disconnect-internet` |
| Patch install-config with mirror settings | Yes | `ipi-conf-mirror`, `mirror-images-by-oc-mirror-conf-mirror` |
| Install OCP cluster | Yes | `ipi-install-install-aws` |
| Run OCP e2e tests | Yes | `openshift-e2e-test` |
| End-to-end workflow combining all of the above | **No** | New: `openshift-e2e-aws-oc-mirror-disconnected` |

---

## Phase 1: Mirror-to-mirror periodic jobs (implemented)

While the true air-gap workflow (phases 2–4 above) is still pending, a mirror-to-mirror
disconnected test has been added as a first step. These jobs use the existing
`openshift-e2e-aws-m2m-disconnected` workflow with `MIRROR_BIN: oc-mirror`, which exercises
oc-mirror v2 to mirror the OCP release payload to the bastion mirror registry before
cluster install. This is a proxy-based disconnected environment (the bastion has internet
access), not a true air-gap, but it is a solid foundation that validates oc-mirror's
mirroring correctness and cluster boot from a mirror.

### How it works

1. `ipi-aws-pre-disconnected` pre-chain: provisions VPC, bastion with mirror registry,
   then calls `mirror-images-by-oc-mirror` with `MIRROR_BIN=oc-mirror` to mirror the OCP
   nightly release payload to the bastion registry using oc-mirror v2.
2. Cluster is installed pointing exclusively at the bastion mirror registry (`ENABLE_IDMS: yes`
   causes an `ImageDigestMirrorSet` to be used instead of the deprecated ICSP).
3. The test step runs OCP e2e tests `openshift-e2e-test`.

### Jobs created

One `__periodics.yaml` was created per actively maintained oc-mirror release branch.
Each job runs weekly on a different weekday to spread cluster provisioning load.

| File | OCP target | Schedule (UTC) | Prow job name |
|---|---|---|---|
| `openshift-oc-mirror-main__periodics.yaml` | OCP 5.0 nightly | Mon 06:00 | `periodic-ci-openshift-oc-mirror-main-periodics-e2e-aws-m2m-disconnected` |
| `openshift-oc-mirror-release-4.23__periodics.yaml` | OCP 4.23 nightly | Tue 06:00 | `periodic-ci-openshift-oc-mirror-release-4.23-periodics-e2e-aws-m2m-disconnected` |
| `openshift-oc-mirror-release-4.22__periodics.yaml` | OCP 4.22 nightly | Wed 06:00 | `periodic-ci-openshift-oc-mirror-release-4.22-periodics-e2e-aws-m2m-disconnected` |
| `openshift-oc-mirror-release-4.21__periodics.yaml` | OCP 4.21 nightly | Thu 06:00 | `periodic-ci-openshift-oc-mirror-release-4.21-periodics-e2e-aws-m2m-disconnected` |
