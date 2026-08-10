# Manual Cases Report

## Summary

- **Total Manual Cases:** 14
- **Test Type:** Functional, Non-functional
- **Type:** Test Case

### Automation Status

| Status      | Count |
|-------------|-------|
| Manual Only | 14     |

---

## Manual Cases Overview

| #  | ID                      | Title                                                                                                    | Importance | Steps |
|----|-------------------------|----------------------------------------------------------------------------------------------------------|------------|-------|
| 1  | [OCP-87356](#ocp-87356) | [v2]Verify that filtered operator catalogs can be successfully mirrored to Nexus registry without manifest conversion errors | Medium     | 3     |
| 2  | [OCP-82938](#ocp-82938) | [OCPBUGS-57461] m2d twice, then d2m in an air-gapped env, no error "trying to fetch from registry.redhat.io" | Medium     | 7     |
| 3  | [OCP-79068](#ocp-79068) | [OCPBUGS-48314] - Verify oc-mirror V2 works on FIPS enabled and STIG compliant RHEL 9 system             | High       | 8     |
| 4  | [OCP-76473](#ocp-76473) | [CLID-75] Validate oc-mirror works well with proxy                                                       | High       | 3     |
| 5  | [OCP-75240](#ocp-75240) | [CLID-55] performance of oc-mirror has been improved with --parallel-images and --parallel-layers flags compared to 4.16 v2 | Low        | 3     |
| 6  | [OCP-75219](#ocp-75219) | [CLID-5] Make sure disk2mirror no need to connect to network [v2]                                        | Critical   | 6     |
| 7  | [OCP-73854](#ocp-73854) | [CLID-5] oc-mirror compatibility test for v2                                                             | Low        | 3     |
| 8  | [OCP-73039](#ocp-73039) | [CLID-5] Support breakpoint resume for v2                                                                | Low        | 3     |
| 9  | [OCP-72991](#ocp-72991) | [CLID-5] Make sure graph image works well for v2                                                         | Critical   | 4     |
| 10 | [OCP-72923](#ocp-72923) | \[CLID-5\]\[CLID-14\] full mirror setting for specified package v2 mirror                                    | High       | 2     |
| 11 | [OCP-72919](#ocp-72919) | \[CLID-5\]\[CLID-14\] full mirror setting for the specified catalog v2 mirror                                | Medium     | 1     |
| 12 | [OCP-72771](#ocp-72771) | [CLID-5] additionalimages support incremetal mirror for v2                                               | Low        | 4     |
| 13 | [OCP-72615](#ocp-72615) | [CLID-5] [CLID-10] [CLID-7] Install an operator  from the mirror registry for V2 format mirror2disk+disk2mirror flow | High       | 6     |
| 14 | [OCP-68764](#ocp-68764) | [WRKLDS-281]Install a OCP cluster from the mirror registry for V2 format mirror2disk+disk2mirror flow using v2 | Critical   | 9     |

---

## Manual Cases Detail

---

### OCP-87356

**[v2]Verify that filtered operator catalogs can be successfully mirrored to Nexus registry without manifest conversion errors.**

| Field | Value |
|-------|-------|
| **ID** | OCP-87356 |
| **Version** | 4.22 |
| **Assignee** | Nidan Gavali (ngavali) |
| **Author** | Nidan Gavali (ngavali) |
| **Importance** | Medium |
| **Test Type** | Functional |

#### Setup

ISC file used:

```yaml
kind: ImageSetConfiguration
apiVersion: mirror.openshift.io/v2alpha1
mirror:
  operators:
  - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.17
    packages:
      - name: 3scale-operator
        channels:
        - name: threescale-2.16
        - name: threescale-2.15
          minVersion: 0.12.2
          maxVersion: 0.12.2
```

**Steps to setup Nexus registry:**

1. Create a podman volume and run Nexus:

```shell
podman volume create nexus-data
podman run -d -p 8081:8081 -p 8082:8082 --name nexus --replace -v nexus-data:/nexus-data:Z sonatype/nexus3:3.37.3
```

2. Access the Nexus UI in your browser at `http://<vm_ip>:8081`
3. Retrieve the admin password:

```shell
podman exec nexus cat /nexus-data/admin.password
```

4. **Create a Docker Repository:** Navigate to Server Administration (Gear Icon) -> Repositories -> Create repository. Select `docker (hosted)`.
5. **Set the Port:** Under "Repository Connectors", check the box for HTTP and enter `8082` in the port field.
6. **Enable the Docker Realm:** Go to Security -> Realms. Move `Docker Bearer Token Realm` to the "Active" column and save.
7. Login using admin:

```shell
podman login --tls-verify=false <vm_ip>:8082 -u admin
```

#### Test Steps

| # | Step | Expected Result |
|---|------|-----------------|
| 1 | Perform mirror2mirror using the command below:<br>oc-mirror -c isc.yaml --workspace file://test docker://<registry_host>:8082 --v2 --dest-tls-verify=false | 2026/02/02 14:15:01  [INFO]   : === Results ===<br>2026/02/02 14:15:01  [INFO]   :  ✓  22 / 22 operator images mirrored successfully<br>2026/02/02 14:15:01  [INFO]   : 📄 Generating IDMS file...<br>2026/02/02 14:15:01  [INFO]   : test/working-dir/cluster-resources/idms-oc-mirror.yaml file created<br>2026/02/02 14:15:01  [INFO]   : 📄 No images by tag were mirrored. Skipping ITMS generation.<br>2026/02/02 14:15:01  [INFO]   : 📄 Generating CatalogSource file...<br>2026/02/02 14:15:01  [INFO]   : test/working-dir/cluster-resources/cs-redhat-operator-index-v4-17.yaml file created<br>2026/02/02 14:15:01  [INFO]   : 📄 Generating ClusterCatalog file...<br>2026/02/02 14:15:01  [INFO]   : test/working-dir/cluster-resources/cc-redhat-operator-index-v4-17.yaml file created<br>2026/02/02 14:15:01  [INFO]   : mirror time     : 46.50941589s<br>2026/02/02 14:15:01  [INFO]   : 👋 Goodbye, thank you for using oc-mirror |
| 2 | podman search --tls-verify=false <vm_ip>:8082/ | NAME                                                  DESCRIPTION<br><registry_host>:8082/3scale-amp2/3scale-operator-bundle  <br><registry_host>:8082/3scale-amp2/3scale-rhel9-operator   <br><registry_host>:8082/3scale-amp2/apicast-gateway-rhel8   <br><registry_host>:8082/3scale-amp2/backend-rhel8           <br><registry_host>:8082/3scale-amp2/manticore-rhel9         <br><registry_host>:8082/3scale-amp2/system-rhel8            <br><registry_host>:8082/3scale-amp2/system-rhel9            <br><registry_host>:8082/3scale-amp2/zync-rhel9              <br><registry_host>:8082/openshift4/ose-cli                  <br><registry_host>:8082/redhat/redhat-operator-index        <br><registry_host>:8082/rhel8/mysql-80                      <br><registry_host>:8082/rhel8/redis-6                       <br><registry_host>:8082/rhel9/memcached                     <br><registry_host>:8082/rhel9/postgresql-15                 <br><registry_host>:8082/rhscl/postgresql-10-rhel7 |
| 3 | Go to browser box>Docker> | All Images are displayed in UI |

---

### OCP-82938

**[OCPBUGS-57461] m2d twice, then d2m in an air-gapped env, no error "trying to fetch from registry.redhat.io"**

| Field | Value |
|-------|-------|
| **ID** | OCP-82938 |
| **Version** | 4.19, 4.18, 4.20, 4.21 |
| **Assignee** | May Xu (maxu) |
| **Author** | May Xu (maxu) |
| **Importance** | Medium |
| **Test Type** | Functional |

#### Setup

1. Create a disconnected cluster (use `functionality-testing/aos-4_19/ipi-on-ibmcloud/versioned-installer-customer_vpc-disconnected-private_cluster` to create the cluster)

2. Prepare `isc.yaml`:

```yaml
kind: ImageSetConfiguration
apiVersion: mirror.openshift.io/v2alpha1
mirror:
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.18
      packages:
       - name: aws-load-balancer-operator
```

3. Prepare `pause.sh` on the bastion host of the cluster:

```bash
#!/bin/bash

WAIT_TIME=60

# === Step 1: Save all current default routes ===
mapfile -t GATEWAYS < <(ip route show default | awk '{print $3}')

if [ ${#GATEWAYS[@]} -eq 0 ]; then
  echo "No default gateways found. Already offline?"
  exit 1
fi

echo "Original Gateways: ${GATEWAYS[@]}"

restore_routes() {
  echo "Restoring internet..."
  for gw in "${GATEWAYS[@]}"; do
    sudo ip route add default via "$gw" 2>/dev/null
  done
  sleep 2
  if ping -c 1 8.8.8.8 &> /dev/null; then
    echo "Internet is now ON."
  else
    echo "Failed to restore internet connectivity."
  fi
}

trap restore_routes EXIT INT TERM

# === Step 2: Cut Off Internet ===
echo "Removing all default routes..."
for gw in "${GATEWAYS[@]}"; do
  sudo ip route del default via "$gw"
done

sleep 2

if ping -c 1 8.8.8.8 &> /dev/null; then
  echo "Warning: Still online after disabling internet!"
else
  echo "Internet is now OFF."
fi

# === Step 3: Wait ===
sleep $WAIT_TIME
```

4. Get the IP of the VM on IBM Cloud:

```shell
ic is ins --all-resource-groups | grep <clusterName>
```

#### Test Steps

| # | Step | Expected Result |
|---|------|-----------------|
| 1 | m2d on the cluster bastion host:<br><br>        `mkdir -p ${XDG_RUNTIME_DIR}/containers`<br>`cp mirror_registry_credentials.json ${XDG_RUNTIME_DIR}/containers/auth.json`<br><br>        `./oc-mirror -c ./isc.yaml file://test --log-level debug --v2`<br><br>        When output "[INFO]   : Start copying the images..." in another terminal, run `./pause.sh` to disable the internet, and then restore the internet. | Mirror to disk failed.<br><br>        2025/06/20 13:36:41  [INFO]   : workflow mode: mirrorToDisk <br><br>        ...<br><br>        2025/06/20 13:36:44  [INFO]   : rebuilding catalogs<br><br>        ...<br><br>        2025/06/20 13:37:52  [INFO]   : Start copying the images...<br><br>        2025/06/20 13:37:52  [INFO]   : images to copy 21 <br><br>        ...<br><br>        2025/06/20 13:39:33  [INFO]   : === Results ===<br><br>        2025/06/20 13:39:33  [INFO]   :  ✗  8 / 21 operator images mirrored: Some operator images failed to be mirrored - please check the logs<br><br>        2025/06/20 13:39:33  [ERROR]  : [Executor] [Worker] some errors occurred during the mirroring. |
| 2 | Rerun m2d twice with the same isc.yaml | mirror to disk succeed.<br><br>        2025/06/20 13:39:42  [INFO]   : workflow mode: mirrorToDisk <br><br>        ...<br><br>        2025/06/20 13:39:42  [INFO]   : rebuilding catalogs<br><br>        2025/06/20 13:39:42  [INFO]   : Start copying the images...<br><br>        2025/06/20 13:39:42  [INFO]   : images to copy 21 <br><br>         ✗   () Rebuilding catalog docker://registry.redhat.io/redhat/redhat-operator-index:v4.18 <br><br>        ...<br><br>        2025/06/20 13:40:01  [INFO]   : === Results ===<br><br>        2025/06/20 13:40:01  [INFO]   :  ✓  21 / 21 operator images mirrored successfully<br><br>        2025/06/20 13:40:01  [DEBUG]  : concurrent channel worker time     : 19.351317722s<br><br>        2025/06/20 13:40:01  [INFO]   : Preparing the tarball archive... |
| 3 | check the disk folder on the bastion host | $ ls test/<br><br>        mirror_000001.tar  working-dir |
| 4 | Transfer files from bastion to the worker node:<br><br>`scp -r test/mirror_000001.tar isc.yaml oc-mirror mirror_registry_credentials.json <worker_node_user>@<worker_node_ip>:~/`<br><br>On the worker node, place the archive into a `test/` directory:<br>`mkdir -p ~/test && mv ~/mirror_000001.tar ~/test/` | Files are copied successfully and `~/test/mirror_000001.tar` exists on the worker node |
| 5 | On the worker node, verify this is an air-gapped env:<br><br>`ping -c 1 8.8.8.8 &> /dev/null; echo $?` | Exit code should be `1` (no connectivity) |
| 6 | On the worker node, set up registry credentials:<br><br>`mkdir -p ${XDG_RUNTIME_DIR}/containers`<br>`cp mirror_registry_credentials.json ${XDG_RUNTIME_DIR}/containers/auth.json` |  |
| 7 | on worker node d2m<br><br>        ./oc-mirror -c isc.yaml --from file://test docker://<mirror_registry>:5000 --v2 --dest-tls-verify=false --parallel-images 8 --retry-delay 2s --retry-times 2 | 2025/06/20 14:47:17  [INFO]   : workflow mode: diskToMirror <br><br>        ...<br><br>        ...<br><br>        2025/06/20 14:52:20  [INFO]   : === Results ===<br><br>        2025/06/20 14:52:20  [INFO]   :  ✓  21 / 21 operator images mirrored successfully<br><br>        2025/06/20 14:52:20  [INFO]   : Generating IDMS file...<br><br>        2025/06/20 14:52:20  [INFO]   : test/working-dir/cluster-resources/idms-oc-mirror.yaml file created<br><br>        2025/06/20 14:52:20  [INFO]   : No images by tag were mirrored. Skipping ITMS generation.<br><br>        2025/06/20 14:52:20  [INFO]   : Generating CatalogSource file...<br><br>        2025/06/20 14:52:20  [INFO]   : test/working-dir/cluster-resources/cs-redhat-operator-index-v4-18.yaml file created<br><br>        2025/06/20 14:52:20  [INFO]   : Generating ClusterCatalog file...<br><br>        2025/06/20 14:52:20  [INFO]   : test/working-dir/cluster-resources/cc-redhat-operator-index-v4-18.yaml file created<br><br>        2025/06/20 14:52:20  [INFO]   : mirror time     : 5m2.71872955s<br><br>        2025/06/20 14:52:20  [INFO]   : Goodbye, thank you for using oc-mirror |

---

### OCP-79068

**[OCPBUGS-48314] - Verify oc-mirror V2 works on FIPS enabled and STIG compliant RHEL 9 system**

| Field | Value |
|-------|-------|
| **ID** | OCP-79068 |
| **Version** | 4.17, 4.18, 4.19 |
| **Assignee** | Kasturi Narra (knarra) |
| **Author** | Kasturi Narra (knarra) |
| **Importance** | High |
| **Test Type** | Functional |

#### Test Steps

| # | Step | Expected Result |
|---|------|-----------------|
| 1 | 1. Login to one of the worker node using the command `oc debug node/<node_name>` ; chroot /host |  |
| 2 | 2. Now open another terminal, login to cluster by copying the login command from the webconsole<br><br>        Browse console url -> login using username & password -> on the top right corner, click on the down arrow -> click copy login command -> click Display Token -> copy paste the token in the terminal | Verify you could login successfully |
| 3 | 3. Now run command `oc get pods -A \| grep debug` and copy the debug pod name. |  |
| 4 | 4. Now copy the oc-mirror binary and isc.yaml to the debug pod, and make it executable:<br>`oc cp oc-mirror <debug_podname>:/tmp/`<br>`oc cp isc.yaml <debug_podname>:/tmp/`<br>`oc exec <debug_podname> -- chmod +x /tmp/oc-mirror` | Verify no error is thrown during the copy |
| 5 | 5. Now to login to the debug pod run command `oc exec -it <debug_pod_name> -- /bin/bash` | Verify login is successful |
| 6 | 6. Run command `ls -l /tmp/` | Verify oc-mirror and isc.yaml that we have copied above are present in the directory |
| 7 | 7.  Run command `/tmp/oc-mirror -c /tmp/isc.yaml file://images --v2 --dry-run`<br><br>        <br><br>kind: ImageSetConfiguration<br>apiVersion: mirror.openshift.io/v2alpha1<br>mirror:<br>  platform:<br>    channels:<br>    - name: stable-4.16<br>      minVersion: 4.16.18<br>      maxVersion: 4.16.24<br>      shortestPath: true | Verify the dry-run command completes successfully. The following non-fatal errors are expected on a FIPS-enabled STIG-compliant system and do not indicate a failure:<br><br>2024/12/18 14:40:01  [INFO]   : going to discover the necessary images...<br>2024/12/18 14:40:01  [INFO]   : collecting release images...<br>2024/12/18 14:40:02  [ERROR]  : openpgp: invalid data: user ID self-signature invalid: openpgp: invalid signature: RSA verification failure (expected on FIPS — non-FIPS signature algorithm rejected)<br>2024/12/18 14:40:02  [ERROR]  : generate release signatures: error list invalid signature for ...sha256:3f14e29f... (expected — same FIPS cause)<br>2024/12/18 14:40:02  [INFO]   : collecting operator images...<br>2024/12/18 14:40:02  [INFO]   : collecting additional images...<br>2024/12/18 14:40:02  [INFO]   : Start copying the images...<br>2024/12/18 14:40:02  [INFO]   : images to copy 0<br>2024/12/18 14:40:02  [INFO]   : === Results ===<br>2024/12/18 14:40:02  [INFO]   : Preparing the tarball archive...<br>2024/12/18 14:40:02  [INFO]   : Goodbye, thank you for using oc-mirror<br>2024/12/18 14:40:02  [ERROR]  : unable to add cache repositories to the archive : lstat .../.oc-mirror/.cache/.../repositories: no such file or directory (expected on first dry-run — cache not yet populated) |
| 8 | 8. Run command `/tmp/oc-mirror -c /tmp/isc.yaml file://images --v2` | Verify that mirror2disk is successful. |

---

### OCP-76473

**[CLID-75] Validate oc-mirror works well with proxy**

| Field | Value |
|-------|-------|
| **ID** | OCP-76473 |
| **Version** | 4.18, 4.19, 4.20, 4.21 |
| **Assignee** | Ying Zhou (yinzhou) |
| **Author** | Ying Zhou (yinzhou) |
| **Importance** | High |
| **Test Type** | Functional |

#### Test Steps

| # | Step | Expected Result |
|---|------|-----------------|
| 1 | Set up the tinyproxy server:<br><br>        1) Install tinyproxy: `sudo dnf install tinyproxy -y`<br><br>        2) Update `/etc/tinyproxy/tinyproxy.conf` with following content:<br><br>        Port 8888<br><br>        Listen 127.0.0.1<br><br>        Timeout 600<br><br>        Allow 127.0.0.1<br><br>        3) Run tinyproxy: `tinyproxy -d -c /etc/tinyproxy/tinyproxy.conf` | Tinyproxy is running successfully |
| 2 | in separate terminal, run oc-mirror with proxy env vars set:<br><br>        `export http_proxy=http://localhost:8888`<br><br>        `export HTTP_PROXY=http://localhost:8888`<br><br>        `export HTTPS_PROXY=http://localhost:8888`<br><br>        `export https_proxy=http://localhost:8888` |  |
| 3 | Save the following imagesetconfig as `config.yaml`:<br><br>        kind: ImageSetConfiguration<br><br>        apiVersion: mirror.openshift.io/v2alpha1<br><br>        mirror:<br><br>          platform:<br><br>            graph: true<br><br>            channels:<br><br>            - name: stable-4.15<br><br>        Verify oc-mirror version: `oc-mirror version`<br><br>        Run mirror2disk:<br>`oc-mirror --config config.yaml file://out --v2` | oc-mirror succeeds. Check the proxy server logs to confirm all requests went through the proxy:<br>`cat /var/log/tinyproxy/tinyproxy.log`<br><br>        <br><br>        CONNECT   Jul 19 09:09:33 [6261]: Established connection to host "api.openshift.com" using file descriptor 10.<br><br>        CONNECT   Jul 19 09:10:30 [6255]: Established connection to host "mirror.openshift.com" using file descriptor 10.<br><br>        CONNECT   Jul 19 09:10:46 [6263]: Established connection to host "quay.io" using file descriptor 10.<br><br>        <br><br>        CONNECT   Jul 19 09:11:14 [6262]: Established connection to host "registry.access.redhat.com" using file descriptor 10.<br><br>        CONNECT   Jul 19 09:11:14 [6262]: Established connection to host "registry.ci.openshift.org" using file descriptor 10. |

---

### OCP-75240

**[CLID-55] performance of oc-mirror has been improved with --parallel-images and --parallel-layers flags compared to 4.16 v2**

| Field | Value |
|-------|-------|
| **ID** | OCP-75240 |
| **Version** | 4.17, 4.18, 4.19 |
| **Assignee** | Ying Zhou (yinzhou) |
| **Author** | Ying Zhou (yinzhou) |
| **Importance** | Low |
| **Test Type** | Non-functional |

#### Test Steps

| # | Step | Expected Result |
|---|------|-----------------|
| 1 | with following imagesetconfig and oc-mirror 4.16 v2 <br><br>        <br><br>        kind: ImageSetConfiguration<br><br>        apiVersion: mirror.openshift.io/v2alpha1<br><br>        mirror:<br><br>          operators:<br><br>            - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.14<br><br>        <br><br>        time `oc-mirror --config config.yaml file://out1 --v2` | record the time |
| 2 | remove the cache directory;<br><br>        with the same imagesetconfig and use oc-mirror 4.17 v2 (defaults: --parallel-images 4, --parallel-layers 5)<br><br>          `time oc-mirror -c config.yaml file://out2 --v2` | Should use less time than the first step. |
| 3 | remove the cache directory;<br><br>        with the same imagesetconfig and increase parallel-images and parallel-layers to compare performance:<br><br>          `time oc-mirror -c config.yaml file://out3 --v2 --parallel-images 8 --parallel-layers 6` | Should use less time than the first step. |

---

### OCP-75219

**[CLID-5] Make sure disk2mirror no need to connect to network [v2]**

| Field | Value |
|-------|-------|
| **ID** | OCP-75219 |
| **Version** | 4.17, 4.16, 4.18, 4.19 |
| **Assignee** | Ying Zhou (yinzhou) |
| **Author** | Ying Zhou (yinzhou) |
| **Importance** | Critical |
| **Test Type** | Functional |

#### Test Steps

| # | Step | Expected Result |
|---|------|-----------------|
| 1 | using follow isc : <br><br>        kind: ImageSetConfiguration<br><br>        apiVersion: mirror.openshift.io/v2alpha1<br><br>        archiveSize: 1<br><br>        mirror:<br><br>          platform:<br><br>            channels:<br><br>            - name: stable-4.15                                             <br><br>            graph: true<br><br>          operators:<br><br>          - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.15<br><br>            packages:<br><br>            - name: quay-bridge-operator<br><br>          additionalImages:<br><br>          - name: registry.redhat.io/rhel8/support-tools:latest<br><br>          - name: quay.io/openshifttest/hello-openshift@sha256:61b8f5e1a3b5dbd9e2c35fd448dc5106337d7a299873dd3a6f0cd8d4891ecc27<br><br>        <br><br>        Do mirror2disk :<br><br>        oc-mirror -c config.yaml file://out  --v2 |  |
| 2 | remove the cache directory |  |
| 3 | copy the archive tar file to a new directory |  |
| 4 | launch a registry by podman :<br><br>        `podman run -d -p 5000:5000 -v /home/registry:/var/lib/registry:Z -e REGISTRY_STORAGE_DELETE_ENABLED=true --restart=always --name registry registry` |  |
| 5 | disable the network for the machine |  |
| 6 | run the disk2mirror command <br><br>        `oc-mirror -c config.yaml --from file://new-path-for-tar  --v2 docker://localhost:5000  --dest-tls-verify=false` | mirror succeed |

---

### OCP-73854

**[CLID-5] oc-mirror compatibility test for v2**

| Field | Value |
|-------|-------|
| **ID** | OCP-73854 |
| **Version** | 4.16, 4.17, 4.18, 4.19 |
| **Assignee** | Nidan Gavali (ngavali) |
| **Author** | Ying Zhou (yinzhou) |
| **Importance** | Low |
| **Test Type** | Functional |

#### Setup

ISC file used:

```yaml
apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
mirror:
  platform:
    channels:
      - name: stable-4.20
        type: ocp
    graph: true
  operators:
    - catalog: registry.redhat.io/redhat/community-operator-index:v4.20
      packages:
        - name: camel-k
          channels:
            - name: stable
            - name: stable-v2
    - catalog: registry.redhat.io/redhat/certified-operator-index:v4.20
      packages:
        - name: crunchy-postgres-operator
          channels:
            - name: v5
  additionalImages:
    - name: quay.io/openshifttest/bench-army-knife@sha256:078db36d45ce0ece589e58e8de97ac1188695ac155bc668345558a8dd77059f6
```

> **Note:** use `--remove-signatures` in case certified catalog is in the ISC

#### Test Steps

| # | Step | Expected Result |
|---|------|-----------------|
| 1 | install latest oc-mirror and oc 4.12 <br>Perform m2m/m2d+d2m | mirror succeed. |
| 2 | install latest oc-mirror and oc 4.13<br>Perform m2m/m2d+d2m | mirror succeed |
| 3 | install latest oc-mirror and oc 4.14 <br>Perform m2m/m2d+d2m | mirror succeed |

---

### OCP-73039

**[CLID-5] Support breakpoint resume for v2**

| Field | Value |
|-------|-------|
| **ID** | OCP-73039 |
| **Version** | 4.16, 4.17, 4.18, 4.19, 4.20, 4.21 |
| **Assignee** | Nidan Gavali (ngavali) |
| **Author** | Ying Zhou (yinzhou) |
| **Importance** | Low |
| **Test Type** | Functional |

#### Test Steps

| # | Step | Expected Result |
|---|------|-----------------|
| 1 | Use following imagesetconfig  do mirror2Disk  , after start to mirror , disrupt the mirror process , then start the mirror again <br> <br> kind: ImageSetConfiguration<br>apiVersion: mirror.openshift.io/v2alpha1<br>archiveSize: 8<br>mirror:<br>  platform:<br>    channels:<br>    - name: stable-4.20<br>      type: ocp<br>      shortestPath: true<br>    graph: true<br>  operators:<br>  - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.20<br>    packages:<br>    - name: advanced-cluster-management<br>      # This moves you to the newest 2026 release cycle<br>      channels:<br>      - name: release-2.15<br>    - name: multicluster-engine<br>      channels:<br>      - name: stable-2.9<br>      - name: stable-2.10<br>    - name: compliance-operator<br>      channels:<br>      - name: stable<br>    - name: self-node-remediation<br>      channels:<br>      - name: stable<br>    - name: jaeger-product<br>      channels:<br>      - name: stable<br>    - name: kiali-ossm<br>      channels:<br>      - name: stable<br>    - name: servicemeshoperator<br>      channels:<br>      - name: stable<br>  additionalImages:<br>  - name: registry.redhat.io/ubi8/ubi:latest                        <br>  - name: registry.redhat.io/rhel8/support-tools:latest<br>  - name: registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.8.0<br>  - name: registry.k8s.io/sig-storage/csi-resizer:v1.8.0<br>  - name: registry.k8s.io/sig-storage/csi-attacher:v4.3.0<br>  - name: registry.k8s.io/sig-storage/csi-provisioner:v3.5.0<br>  - name: registry.k8s.io/sig-storage/csi-snapshotter:v6.2.2<br>  - name: registry.access.redhat.com/ubi8/nginx-120:latest<br>  - name: registry.gitlab.com/gitlab-org/build/cng/kubectl:v16.5.1<br><br><br> <br> `oc-mirror --config config.yaml file://catchtest --v2` | record the mirror time :<br> <br> 2026/01/16 11:15:20  [INFO]   : === Results ===<br>2026/01/16 11:15:20  [INFO]   :  ✓  194 / 194 release images mirrored successfully<br>2026/01/16 11:15:20  [INFO]   :  ✓  192 / 192 operator images mirrored successfully<br>2026/01/16 11:15:20  [INFO]   :  ✓  9 / 9 additional images mirrored successfully<br>2026/01/16 11:15:20  [INFO]   : 📦 Preparing the tarball archive...<br>2026/01/16 11:21:48  [INFO]   : mirror time     : 17m34.609817328s<br>2026/01/16 11:21:48  [INFO]   : 👋 Goodbye, thank you for using oc-mirror |
| 2 | Delete the local cache and the local registry, use following imagesetconfig  do mirror2Disk <br> rm -rf ~/.oc-mirror<br> <br> kind: ImageSetConfiguration<br>apiVersion: mirror.openshift.io/v2alpha1<br>archiveSize: 8<br>mirror:<br>  platform:<br>    channels:<br>    - name: stable-4.20<br>      type: ocp<br>      shortestPath: true<br>    graph: true<br>  operators:<br>  - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.20<br>    packages:<br>    - name: advanced-cluster-management<br>      # This moves you to the newest 2026 release cycle<br>      channels:<br>      - name: release-2.15<br>    - name: multicluster-engine<br>      channels:<br>      - name: stable-2.9<br>      - name: stable-2.10<br>    - name: compliance-operator<br>      channels:<br>      - name: stable<br>    - name: self-node-remediation<br>      channels:<br>      - name: stable<br>    - name: jaeger-product<br>      channels:<br>      - name: stable<br>    - name: kiali-ossm<br>      channels:<br>      - name: stable<br>    - name: servicemeshoperator<br>      channels:<br>      - name: stable<br>  additionalImages:<br>  - name: registry.redhat.io/ubi8/ubi:latest                        <br>  - name: registry.redhat.io/rhel8/support-tools:latest<br>  - name: registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.8.0<br>  - name: registry.k8s.io/sig-storage/csi-resizer:v1.8.0<br>  - name: registry.k8s.io/sig-storage/csi-attacher:v4.3.0<br>  - name: registry.k8s.io/sig-storage/csi-provisioner:v3.5.0<br>  - name: registry.k8s.io/sig-storage/csi-snapshotter:v6.2.2<br>  - name: registry.access.redhat.com/ubi8/nginx-120:latest<br>  - name: registry.gitlab.com/gitlab-org/build/cng/kubectl:v16.5.1<br> <br> <br> `oc-mirror --config config.yaml file://catchtest --v2` | When the second mirror process , disrupt it ,and start the third mirror .<br> wait for mirror completed , check the mirror time again :<br> <br> 2026/01/16 12:30:04  [INFO]   : === Results ===<br>2026/01/16 12:30:04  [INFO]   :  ✓  194 / 194 release images mirrored successfully<br>2026/01/16 12:30:04  [INFO]   :  ✓  192 / 192 operator images mirrored successfully<br>2026/01/16 12:30:04  [INFO]   :  ✓  9 / 9 additional images mirrored successfully<br>2026/01/16 12:30:04  [INFO]   : 📦 Preparing the tarball archive...<br>2026/01/16 12:30:29  [INFO]   : mirror time     : 19m15.627714718s<br>2026/01/16 12:30:29  [INFO]   : 👋 Goodbye, thank you for using oc-mirror |
| 3 | Delete the local registry, keep the cache and run m2d | 2026/01/16 12:49:07  [INFO]   : === Results ===<br>2026/01/16 12:49:07  [INFO]   :  ✓  194 / 194 release images mirrored successfully<br>2026/01/16 12:49:07  [INFO]   :  ✓  192 / 192 operator images mirrored successfully<br>2026/01/16 12:49:07  [INFO]   :  ✓  9 / 9 additional images mirrored successfully<br>2026/01/16 12:49:07  [INFO]   : 📦 Preparing the tarball archive...<br>2026/01/16 12:55:28  [INFO]   : mirror time     : 17m47.116322967s<br>2026/01/16 12:55:28  [INFO]   : 👋 Goodbye, thank you for using oc-mirror<br><br>Observe how third mirror took less time compared to the second time. |

---

### OCP-72991

**[CLID-5] Make sure graph image works well for v2**

| Field | Value |
|-------|-------|
| **ID** | OCP-72991 |
| **Version** | 4.16, 4.17, 4.18, 4.19 |
| **Assignee** | Nidan Gavali (ngavali) |
| **Author** | Ying Zhou (yinzhou) |
| **Importance** | Critical |
| **Test Type** | Functional |

#### Test Steps

| # | Step | Expected Result |
|---|------|-----------------|
| 1 | Install disconnected cluster with version 4.21 |  |
| 2 | cat config-ocp.yaml <br> kind: ImageSetConfiguration<br>apiVersion: mirror.openshift.io/v2alpha1<br>mirror:<br>  platform:<br>    channels:<br>    - name: stable-4.20<br>      type: ocp<br>      minVersion: '4.20.0'<br>      maxVersion: '4.20.4' <br>      shortestPath: true<br>    graph: true<br>  operators:<br>  - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.20<br>    packages:<br>    - name: cincinnati-operator<br>      channels:<br>      - name: v1<br><br> `oc-mirror --config config-ocp.yaml file://ocp  --v2`<br> `oc-mirror --config config-ocp.yaml --from file://ocp docker://xxx.com:5000/ocp  --v2` | mirror succeed, since we have set shortpath==true , make sure we only mirror the minimum upgradable path (not contain all the available version) |
| 3 | install OpenShift Update Service operator for the cluster |  |
| 4 | Create the UpdateService resource:<br> `oc create -f ocp/working-dir/cluster-resources/updateService.yaml -n openshift-update-service` | Make sure all pods are running:<br> `oc get pod -n openshift-update-service`<br> NAME                                      READY   STATUS    RESTARTS   AGE<br> graph-data-tag-digest                     1/1     Running   0          23s<br> update-service-oc-mirror-d9b7c9cf-92cdf   2/2     Running   0          10m<br> update-service-oc-mirror-d9b7c9cf-vv94h   2/2     Running   0          10m |

---

### OCP-72923

**\[CLID-5\]\[CLID-14\] full mirror setting for specified package v2 mirror**

| Field | Value |
|-------|-------|
| **ID** | OCP-72923 |
| **Version** | 4.16, 4.17, 4.18, 4.19 |
| **Assignee** | Ying Zhou (yinzhou) |
| **Author** | Ying Zhou (yinzhou) |
| **Importance** | High |
| **Test Type** | Functional |

#### Test Steps

| # | Step | Expected Result |
|---|------|-----------------|
| 1 | kind: ImageSetConfiguration<br>apiVersion: mirror.openshift.io/v2alpha1<br>mirror:<br>  operators:<br>    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.20<br>      full: false <br>      packages: <br>        - name: devworkspace-operator<br>        - name: 3scale-operator<br>          defaultChannel: threescale-mas<br>          channels:<br>            - name: threescale-mas<br> <br> `oc-mirror --config config-f.yaml file://out --v2`<br> `oc-mirror --config config-f.yaml --from file://out --v2 docker://xxx/yy` | Will mirror all bundles of all channels for devworkspace-operator<br> <br> Will mirror all bundles of the specified channel threescale-mas for operator 3scale-operator |
| 2 | install the catalogsource and operator | catalog source and operator install well , check the package manifest , the versions info is expected |

---

### OCP-72919

**\[CLID-5\]\[CLID-14\] full mirror setting for the specified catalog v2 mirror**

| Field | Value |
|-------|-------|
| **ID** | OCP-72919 |
| **Version** | 4.16, 4.17, 4.18, 4.19 |
| **Assignee** | Ying Zhou (yinzhou) |
| **Author** | Ying Zhou (yinzhou) |
| **Importance** | Medium |
| **Test Type** | Functional |

#### Test Steps

| # | Step | Expected Result |
|---|------|-----------------|
| 1 | Use any of the ISC below:<br>ISC1:<br>kind: ImageSetConfiguration<br> apiVersion: mirror.openshift.io/v2alpha1<br> mirror:<br>   operators:<br>     - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.14<br>       full: true<br> <br>ISC2:<br>kind: ImageSetConfiguration<br>apiVersion: mirror.openshift.io/v2alpha1<br>mirror:<br>  operators:<br>    - catalog: quay.io/oc-mirror/oc-mirror-dev:test-catalog-latest<br>      full: true<br><br> Do mirror2Disk and disk2Mirror actions | will mirror all bundles (versions)  of all channels of the specified catalog |

---

### OCP-72771

**[CLID-5] additionalimages support incremetal mirror for v2**

| Field | Value |
|-------|-------|
| **ID** | OCP-72771 |
| **Version** | 4.16, 4.17, 4.18, 4.19, 4.20, 4.21 |
| **Assignee** | Nidan Gavali (ngavali) |
| **Author** | Ying Zhou (yinzhou) |
| **Importance** | Low |
| **Test Type** | Functional |

#### Test Steps

| # | Step | Expected Result |
|---|------|-----------------|
| 1 | Use the first imagesetconfigure to mirror<br> cat config-f.yaml<br> kind: ImageSetConfiguration<br> apiVersion: mirror.openshift.io/v2alpha1<br> mirror:<br>   additionalImages:<br>   - name: registry.k8s.io/sig-storage/csi-provisioner:v3.5.0<br>   - name: registry.k8s.io/sig-storage/csi-snapshotter:v6.2.2<br>   - name: registry.access.redhat.com/ubi8/nginx-120:latest<br>   - name: registry.gitlab.com/gitlab-org/build/cng/kubectl:v16.5.1<br>   - name: quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83<br>   - name: quay.io/openshifttest/hello-openshift@sha256:61b8f5e1a3b5dbd9e2c35fd448dc5106337d7a299873dd3a6f0cd8d4891ecc27<br>   helm: {}<br> <br> `oc-mirror --config config-f.yaml  file://out --v2` | mirror succeed |
| 2 | move the tar file to back directory |  |
| 3 | Use the second imagesetconfigure to mirror <br> kind: ImageSetConfiguration<br> apiVersion: mirror.openshift.io/v2alpha1<br> mirror:<br>   additionalImages:<br>   - name: registry.redhat.io/ubi8/ubi:latest                        <br>   - name: registry.redhat.io/rhel8/support-tools:latest<br>   - name: registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.8.0<br>   - name: registry.k8s.io/sig-storage/csi-resizer:v1.8.0<br>   - name: registry.k8s.io/sig-storage/csi-attacher:v4.3.0<br>   - name: registry.k8s.io/sig-storage/csi-provisioner:v3.5.0<br>   - name: registry.k8s.io/sig-storage/csi-snapshotter:v6.2.2<br>   - name: registry.access.redhat.com/ubi8/nginx-120:latest<br>   - name: registry.gitlab.com/gitlab-org/build/cng/kubectl:v16.5.1<br>   - name: quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83<br>   - name: quay.io/openshifttest/hello-openshift@sha256:61b8f5e1a3b5dbd9e2c35fd448dc5106337d7a299873dd3a6f0cd8d4891ecc27<br>   helm: {}<br> <br> <br> `oc-mirror --config config-s.yaml  file://out --v2` | mirror succeed |
| 4 | untar the first and second tar file to different dir , compare the content from docker/registry/v2/blobs/sha256 using compare_digest.sh | ./compare_digest.sh tar1/docker/registry/v2/blobs/sha256/ tar2/docker/registry/v2/blobs/sha256/<br>---------------------------------------------------<br>Directory 1 Digest: 3e866d1eef12b942c8d778290648e4b64a322f1391f0d95e3ee554f6de59b0da<br>Directory 2 Digest: ee8e510fcaf8e4f7b981632c68344ffaac7d90ef2320a3753165f79535a16e26<br>---------------------------------------------------<br>RESULT: The directories are DIFFERENT. |

---

### OCP-72615

**[CLID-5] [CLID-10] [CLID-7] Install an operator  from the mirror registry for V2 format mirror2disk+disk2mirror flow**

| Field | Value |
|-------|-------|
| **ID** | OCP-72615 |
| **Version** | 4.16, 4.17, 4.18, 4.19 |
| **Assignee** | Ying Zhou (yinzhou) |
| **Author** | Ying Zhou (yinzhou) |
| **Importance** | High |
| **Test Type** | Functional |

#### Test Steps

| # | Step | Expected Result |
|---|------|-----------------|
| 1 | kind: ImageSetConfiguration<br>apiVersion: mirror.openshift.io/v2alpha1<br>mirror:<br>  platform:<br>    channels:<br>    - name: stable-4.20<br>      type: ocp<br>      minVersion: '4.20.0'<br>      maxVersion: '4.20.0'<br>      shortestPath: true<br>    graph: true<br>  operators:<br>  - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.20<br>    packages:<br>    - name: advanced-cluster-management<br>      channels:<br>      - name: release-2.15<br>    - name: compliance-operator<br>      channels:<br>      - name: stable<br>    - name: multicluster-engine<br>      channels:<br>      - name: stable-2.10<br>  additionalImages:<br>  - name: registry.redhat.io/ubi8/ubi:latest<br>  - name: registry.redhat.io/rhel8/support-tools:latest<br>  - name: registry.access.redhat.com/ubi8/nginx-120:latest<br>  - name: registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.8.0<br>  - name: registry.k8s.io/sig-storage/csi-resizer:v1.8.0<br>  - name: quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83<br>  - name: quay.io/openshifttest/hello-openshift@sha256:61b8f5e1a3b5dbd9e2c35fd448dc5106337d7a299873dd3a6f0cd8d4891ecc27<br><br><br> <br> <br> `oc-mirror --config config-filter.yaml file://outfilter   --v2` | mirror succeed |
| 2 | Back up the mirrored archive for reference:<br>`cp -r outfilter outfilter-backup` |  |
| 3 | cat config-filter.yaml <br> kind: ImageSetConfiguration<br>apiVersion: mirror.openshift.io/v2alpha1<br>mirror:<br>  platform:<br>    channels:<br>    - name: stable-4.20<br>      type: ocp<br>      minVersion: '4.20.0'<br>      maxVersion: '4.20.0'<br>      shortestPath: true<br>    graph: true<br>  operators:<br>  - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.20<br>    packages:<br>    - name: advanced-cluster-management<br>      channels:<br>      - name: release-2.15<br>    - name: compliance-operator<br>      channels:<br>      - name: stable<br>    - name: multicluster-engine<br>      channels:<br>      - name: stable-2.10<br>  additionalImages:<br>  - name: registry.redhat.io/ubi8/ubi:latest<br>  - name: registry.redhat.io/rhel8/support-tools:latest<br>  - name: registry.access.redhat.com/ubi8/nginx-120:latest<br>  - name: registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.8.0<br>  - name: registry.k8s.io/sig-storage/csi-resizer:v1.8.0<br>  - name: quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83<br>  - name: quay.io/openshifttest/hello-openshift@sha256:61b8f5e1a3b5dbd9e2c35fd448dc5106337d7a299873dd3a6f0cd8d4891ecc27<br><br>  <br> <br> `oc-mirror -c config-filter.yaml --from file://outfilter --v2 docker://localhost:5000 --dest-tls-verify=false` | filter data succeed |
| 4 | Run disk2mirror to remote registry:<br> `oc-mirror --config config-filter.yaml --from file://outfilter  docker://<bastion_host>:5000/ocpv2  --v2` | mirror succeed, check the IDMS / ITMS / CatalogSource file created correctly<br> note : will generate CatalogSource for each operator catalog |
| 5 | Create the IDMS \ITMS and catalogsource file | catalog source created succeed |
| 6 | Install the operator from the registry | operator installed , no issue |

---

### OCP-68764

**[WRKLDS-281]Install a OCP cluster from the mirror registry for V2 format mirror2disk+disk2mirror flow using v2**

| Field | Value |
|-------|-------|
| **ID** | OCP-68764 |
| **Version** | 4.15, 4.16, 4.17, 4.18, 4.19 |
| **Assignee** | Ying Zhou (yinzhou) |
| **Author** | Ying Zhou (yinzhou) |
| **Importance** | Critical |
| **Test Type** | Functional |

<details>
<summary>new-install-config.yaml template (used in step 5)</summary>

> **This is a template, not a runnable file.** All `<placeholder>` values below must be replaced with your environment-specific values before running `openshift-install create cluster`.

| Placeholder | Description |
|---|---|
| `<cluster_name>` | Name for the new cluster |
| `<subnet_id_1>` ... `<subnet_id_4>` | VPC subnet IDs from the disconnected environment |
| `<bastion_host>` | Hostname or IP of the bastion / mirror registry host |
| `<base64_credentials>` | Base64-encoded `username:password` for the mirror registry |
| `<your_base_domain>` | DNS base domain for the cluster |
| `<your_ssh_public_key>` | Full SSH public key (e.g. `ssh-rsa AAAA...`) |
| `additionalTrustBundle` | Replace with the full PEM certificate of the mirror registry CA |

```yaml
apiVersion: v1
controlPlane:
  architecture: amd64
  hyperthreading: Enabled
  name: master
  platform: {}
  replicas: 3
compute:
- architecture: amd64
  hyperthreading: Enabled
  name: worker
  platform: {}
  replicas: 3
metadata:
  name: <cluster_name>                  # REPLACE
platform:
  aws:
    region: us-east-2
    subnets:                             # REPLACE all subnet IDs
    - <subnet_id_1>
    - <subnet_id_2>
    - <subnet_id_3>
    - <subnet_id_4>
pullSecret: '{"auths":{"<bastion_host>:5000":{"auth":"<base64_credentials>"}}}'  # REPLACE
networking:
  clusterNetwork:
  - cidr: 10.128.0.0/14
    hostPrefix: 23
  serviceNetwork:
  - 172.30.0.0/16
  machineNetwork:
  - cidr: 10.0.0.0/16
  networkType: OpenShiftSDN
publish: External
additionalTrustBundle: |               # REPLACE with mirror registry CA cert
  -----BEGIN CERTIFICATE-----
  <paste full PEM certificate here>
  -----END CERTIFICATE-----
proxy:
  httpProxy: http://<bastion_host>:3128   # REPLACE
  httpsProxy: http://<bastion_host>:3128  # REPLACE
  noProxy: <bastion_host>,ec2.<region>.amazonaws.com,elasticloadbalancing.<region>.amazonaws.com,s3.<region>.amazonaws.com,.s3.<region>.amazonaws.com,.s3.dualstack.<region>.amazonaws.com,sts.<region>.amazonaws.com,iam.amazonaws.com,route53.amazonaws.com,tagging.<region>.amazonaws.com,.<your_base_domain>,10.0.0.0/16,10.128.0.0/14,172.30.0.0/16  # REPLACE with your environment values
imageContentSources:                     # REPLACE bastion_host in mirrors
- mirrors:
  - <bastion_host>:5000/ocpv2/openshift-release-dev
  source: quay.io/openshift-release-dev
- mirrors:
  - <bastion_host>:5000/ocpv2/openshift
  source: localhost:5000/openshift
baseDomain: <your_base_domain>           # REPLACE
sshKey: <your_ssh_public_key>            # REPLACE
```

</details>

#### Test Steps

| # | Step | Expected Result |
|---|------|-----------------|
| 1 | Use the template to launch a disconnected cluster :<br><br>          aos-4_14/ipi-on-aws/versioned-installer-customer_vpc-disconnected |  |
| 2 | Like  imageset.yaml , push 4.13.17 ocp image to the registry server of the bastion host:<br><br>          apiVersion: mirror.openshift.io/v1alpha2<br><br>          kind: ImageSetConfiguration<br><br>          mirror:<br><br>            platform:<br><br>              channels:<br><br>                - name: stable-4.13<br><br>                  minVersion: 4.13.17<br><br>                  maxVersion: 4.13.18<br><br>              graph: true<br><br>          <br><br>          <br><br>          `oc-mirror --config config-filter.yaml file://outfilter   --v2` |  |
| 3 | cat config-filter.yaml <br><br>        apiVersion: mirror.openshift.io/v1alpha2<br><br>        kind: ImageSetConfiguration<br><br>        mirror:<br><br>          platform:<br><br>            channels:<br><br>              - name: stable-4.13<br><br>                minVersion: 4.13.17<br><br>                maxVersion: 4.13.17<br><br>            graph: true<br><br>        <br><br>        `oc-mirror --v2 prepare --from file://outfilter --config config-filter.yaml file://outfilter -p 5005` |  |
| 4 | mirror the image to remote registry by command :<br><br>        `oc-mirror --config config-filter.yaml --from file://outfilter  docker://<bastion_host>:5000/ocpv2  --v2` |  |
| 5 | Copy the install-config.yaml from Flexy-install, update the `imageContentSources` to point to the mirror registry (refer to oc-mirror IDMS output). Use the install-config template above. |  |
| 6 | Extract openshift-install from quay.io:<br>`oc adm release extract --tools quay.io/openshift-release-dev/ocp-release:4.13.17-x86_64` |  |
| 7 | `./openshift-install create cluster` to launch the second cluster using the mirror image |  |
| 8 | SSH into the bastion, add the second cluster's domain to proxy whitelist setting:<br><br>        /srv/squid/etc/squid.conf<br><br>          acl whitelist dstdomain <bastion_host> tagging.<region>.amazonaws.com route53.amazonaws.com ec2.<region>.amazonaws.com iam.amazonaws.com .s3.dualstack.<region>.amazonaws.com .s3.<region>.amazonaws.com elasticloadbalancing.<region>.amazonaws.com .apps.<cluster1_name>.qe.devcluster.openshift.com .github.com .rubygems.org sts.amazonaws.com sts.<region>.amazonaws.com .apps.<cluster2_name>.qe.devcluster.openshift.com<br><br>        `sudo systemctl restart squid-proxy.service` |  |
| 9 | Verify the second cluster is healthy:<br><br>`oc get nodes`<br>`oc get co`<br>`oc get clusterversion` | All nodes should be in `Ready` status.<br>All cluster operators should be `Available=True`, `Progressing=False`, `Degraded=False`.<br>`oc get clusterversion` should show the expected version (e.g. 4.13.17) with `Available=True`. |

---