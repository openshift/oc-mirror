# Overview

## POC implementation of file based oci and docker v2 format for mirroring

**NB** This is still a WIP the cli and suggested implementation is continuously under change

This is a simpler implementation of the functionality found in oc-mirror

As mentioned its still very raw (hence POC) but has the following functionality

Images use oci format where possible (except for release as they have been created in v2docker2 format)

- The release images need to preserve format and digest (important for installing in disconnected envrionments)

- cincinnati client (release and channel versioning)
- release downloads to disk from registry
- operator downloads to disk from registry
- additionalImages to disk from registry

- release upload to registry from disk
- operator upload to registry from disk
- additionalImages to registry from disk

At this point in time the problems/lack of features are 
- mirrorTomirror functionality (needs chain from mirrorToDisk and diskToMirror)
- limited tests
- detached api's - need to implement for upstream api changes
- no e2e tests


## Usage

For the mirror-to-disk use case

``` bash

mirror oci:test-dir --config isc.yaml --loglevel debug

# imagesetconfig used

---
apiVersion: mirror.openshift.io/v1alpha2
kind: ImageSetConfiguration
storageConfig:
  local:
    path: /tmp/lmz-images
mirror:
  platform:
    architectures:
      - "amd64"
    channels:
      - name: stable-4.12
        minVersion: 4.12.0
        maxVersion: 4.12.0
  operators:
    #- catalog: oci:///home/lzuccarelli/go/src/github.com/openshift/oc-mirror/newlmz/redhat-operator-index
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.12
      packages:
      - name: aws-load-balancer-operator
        channels: 
        - name: stable-v0
  
  additionalImages: 
    #- name: registry.redhat.io/ubi8/ubi:latest  
    - name: registry.redhat.io/ubi9/ubi:latest

```

For the disk-to-mirror use case 

```bash

# cli

mirror docker://localhost:5000/test --config isc-mirror.yaml --loglevel trace

# imagesetconfig used

---
apiVersion: mirror.openshift.io/v1alpha2
kind: ImageSetConfiguration
mirror:
  platform:
    release: dir:///home/lzuccarelli/go/src/github.com/lmzuccarelli/golang-fb-mirror/working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64
  operators:
    - catalog: dir:///home/lzuccarelli/go/src/github.com/lmzuccarelli/golang-fb-mirror/working-dir/test-lmz/operator-images/redhat-operator-index/v4.12
      packages:
      - name: aws-load-balancer-operator
        channels: 
        - name: stable-v0
  additionalImages: 
    - name: dir:///home/lzuccarelli/go/src/github.com/lmzuccarelli/golang-fb-mirror/working-dir/test-lmz/additional-images
    
```

## Profiling 

The main performance gain here has been the disk-to-mirror 

For example (see logs below) the time taken to mirror 189 images (releases,operators and additionalImages)
was about 100 seconds.

I measured the same using the oc-mirror legacy (183 images)  and it took about 20 minutes

```bash
[rossonero Fri Mar 24 14:53:38 lzuccarelli] ~/go/src/github.com/lmzuccarelli/golang-fb-mirror {main}
$ build/mirror docker://localhost.localdomain:5000/testlmz --release-from working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/ --operators-from working-dir/test-lmz/operator-images/redhat-operator-index/v4.12/ --additional-from working-dir/test-lmz/additional-images/ --loglevel debug
2023/03/24 15:00:16  [DEBUG]  : imagesetconfig file  
2023/03/24 15:00:16  [ERROR]  : imagesetconfig read .: is a directory 
2023/03/24 15:00:16  [INFO]   : mode diskToMirror 
2023/03/24 15:00:16  [INFO]   : added new metadata {golang-fb-mirror CFE-EAMA {[{0 1679666416  true working-dir}]}} 
2023/03/24 15:00:16  [ERROR]  : open /.metadata.toml: permission denied
2023/03/24 15:00:16  [INFO]   : metadata {golang-fb-mirror CFE-EAMA {[{0 1679666416  false working-dir}]}} 
2023/03/24 15:00:16  [INFO]   : total release images to copy 183 
2023/03/24 15:00:16  [INFO]   : path working-dir/test-lmz/operator-images/redhat-operator-index/v4.12/aws-load-balancer-operator.v0.2.0/albo/aws-load-balancer-rhel8-operator-ab38b37c14f7f0897e09a18eca4a232a6c102b76e9283e401baed832852290b5-annotation
2023/03/24 15:00:16  [INFO]   : path working-dir/test-lmz/operator-images/redhat-operator-index/v4.12/aws-load-balancer-operator.v0.2.0/albo/cb36fc
2023/03/24 15:00:16  [INFO]   : path working-dir/test-lmz/operator-images/redhat-operator-index/v4.12/aws-load-balancer-operator.v0.2.0/albo/controller
2023/03/24 15:00:16  [INFO]   : path working-dir/test-lmz/operator-images/redhat-operator-index/v4.12/aws-load-balancer-operator.v0.2.0/albo/manager
2023/03/24 15:00:16  [INFO]   : path working-dir/test-lmz/operator-images/redhat-operator-index/v4.12/aws-load-balancer-operator.v0.2.0/openshift4/kube-rbac-proxy
2023/03/24 15:00:16  [INFO]   : total operator images to copy 5 
2023/03/24 15:00:16  [INFO]   : total additional images to copy 1 
2023/03/24 15:00:16  [INFO]   : images to mirror 189 
2023/03/24 15:00:16  [INFO]   : batch count 23 
2023/03/24 15:00:16  [INFO]   : batch index 0 
2023/03/24 15:00:16  [INFO]   : batch size 8 
2023/03/24 15:00:16  [INFO]   : remainder size 5 
2023/03/24 15:00:16  [INFO]   : starting batch 0 
2023/03/24 15:00:16  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/agent-installer-api-server 
2023/03/24 15:00:16  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:f30638f60452062aba36a26ee6c036feead2f03b28f2c47f2b0a991e4182331e 
2023/03/24 15:00:16  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/agent-installer-csr-approver 
2023/03/24 15:00:16  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:1511693daf5d1cb36507c8f17c6b77b00266a8815e6f204c3243f4677128db61 
2023/03/24 15:00:16  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/agent-installer-node-agent 
2023/03/24 15:00:16  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:955faaa822dc107f4dffa6a7e457f8d57a65d10949f74f6780ddd63c115e31e5 
2023/03/24 15:00:16  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/agent-installer-orchestrator 
2023/03/24 15:00:16  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:4949b93b3fd0f6b22197402ba22c2775eba408b53d30ac2e3ab2dda409314f5e 
2023/03/24 15:00:16  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/alibaba-cloud-controller-manager 
2023/03/24 15:00:16  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:bd3c588a15b688b9568d8687e81e2af69a9ee9e87c1f5de2003c881e51544bb7 
2023/03/24 15:00:16  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/alibaba-cloud-csi-driver 
2023/03/24 15:00:16  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:f02f76546aa186ed6516417d4298b820d269516ae6278e339c94c0277cbe8580 
2023/03/24 15:00:16  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/alibaba-disk-csi-driver-operator 
2023/03/24 15:00:16  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:98ed09f9be29ab5b1055e118ab98687eb09fbaa5fbde2d41fc20eb8da7b544f1 
2023/03/24 15:00:16  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/alibaba-machine-controllers 
2023/03/24 15:00:16  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:341df5d61566d6ef4282651284535145f50f4fcdad94e8b206a7576118c7785a 
2023/03/24 15:00:19  [INFO]   : completed batch 0
2023/03/24 15:00:19  [INFO]   : starting batch 1 
2023/03/24 15:00:19  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/apiserver-network-proxy 
2023/03/24 15:00:19  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:2a0dd75b1b327a0c5b17145fc71beb2bf805e6cc3b8fc3f672ce06772caddf21 
2023/03/24 15:00:19  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/aws-cloud-controller-manager 
2023/03/24 15:00:19  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:aa99f4bbbfb26f266ff53cb3e85e1298b2a52b28a7d70dc1dbd36003e1fa3dc1 
2023/03/24 15:00:19  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/aws-cluster-api-controllers 
2023/03/24 15:00:19  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:5bcee2b0cf9c1275fe7b048f96e9b345ddd3ff5c31e3072310e6775ad1a2eaed 
2023/03/24 15:00:19  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/aws-ebs-csi-driver 
2023/03/24 15:00:19  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:6d6b0a4a7f205d5e52404797b190b7ef5063a999f3f91680e02229b22790b916 
2023/03/24 15:00:19  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/aws-ebs-csi-driver-operator 
2023/03/24 15:00:19  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:4e596cc1ded888a6fb4e8cbb9135b18b016ab8edcca1c0818d0231f7ec9a8908 
2023/03/24 15:00:19  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/aws-machine-controllers 
2023/03/24 15:00:19  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:ef9db227070ef68b463ed902ff609cb8ae2859aedf30d69367f0feae248832c9 
2023/03/24 15:00:19  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/aws-pod-identity-webhook 
2023/03/24 15:00:19  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:ca1ce278e33d80952a1203209ac45f0ed2fb79907a7e7c135d1af0d14c4b4cdb 
2023/03/24 15:00:19  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/azure-cloud-controller-manager 
2023/03/24 15:00:19  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:09c8aa056bbf5da5c7119653d2000f9412b6d2f48304911e3d2913cad5f29ef4 
2023/03/24 15:00:20  [INFO]   : completed batch 1
2023/03/24 15:00:20  [INFO]   : starting batch 2 
2023/03/24 15:00:20  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/azure-cloud-node-manager 
2023/03/24 15:00:20  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:f266a2674a945022a377e2417631bb6d581e8b265366d0e007c52f105b6d5b6b 
2023/03/24 15:00:20  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/azure-cluster-api-controllers 
2023/03/24 15:00:20  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:5e8660a2d4a098d897a3422495158d6d4ba4527378df79838889d1d87a9a3c53 
2023/03/24 15:00:20  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/azure-disk-csi-driver 
2023/03/24 15:00:20  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:af1f30a0f3480ac0622da94639a09de309175f8b97adbf70426dd57a558dfb43 
2023/03/24 15:00:20  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/azure-disk-csi-driver-operator 
2023/03/24 15:00:20  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:c5c3f5b2c958ccb083a36414baa947580291be035053864da379c1cf131bf1ce 
2023/03/24 15:00:20  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/azure-file-csi-driver 
2023/03/24 15:00:20  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:ebfadb579d329d6ba3297bfafb66cf41ea5d84ea61523273792e7e6337f4f2fa 
2023/03/24 15:00:20  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/azure-file-csi-driver-operator 
2023/03/24 15:00:20  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:d8309eb45097f821a093a912212d8571a4f79221474daba24c5c21e62c587ec5 
2023/03/24 15:00:20  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/azure-machine-controllers 
2023/03/24 15:00:20  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:c73efc9a5ffe8d209aee3b12564c1ff707370606255716b7cf0f94c130ba4b62 
2023/03/24 15:00:20  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/baremetal-installer 
2023/03/24 15:00:20  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:1631b0f0bf9c6dc4f9519ceb06b6ec9277f53f4599853fcfad3b3a47d2afd404 
2023/03/24 15:00:21  [INFO]   : completed batch 2
2023/03/24 15:00:21  [INFO]   : starting batch 3 
2023/03/24 15:00:21  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/baremetal-machine-controllers 
2023/03/24 15:00:21  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:49619264429ace25f813b97f0d7464374266a70a1a5f36614658c0bcf514a1bb 
2023/03/24 15:00:21  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/baremetal-operator 
2023/03/24 15:00:21  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:e6f0b3103d68b12a1bf4b9058f60e9a7de52c27f58cab199c9000fdbc754c2c3 
2023/03/24 15:00:21  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/baremetal-runtimecfg 
2023/03/24 15:00:21  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:6b61429030b790e6ec6e9fcb52b2a17c5b794815d6da9806bc563bc45e84aa67 
2023/03/24 15:00:21  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cli 
2023/03/24 15:00:21  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:1fc458ece66c8d4184b45b5c495a372a96b47432ae5a39844cd5837e3981685b 
2023/03/24 15:00:21  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cli-artifacts 
2023/03/24 15:00:21  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:2823d5f6c6f9145b1c817f20ebfb1be4b72656086d58182356e911c324efebaf 
2023/03/24 15:00:21  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cloud-credential-operator 
2023/03/24 15:00:21  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:e2981cdba6d1e6787c1b5b048bba246cc307650a53ef680dc44593e6227333f1 
2023/03/24 15:00:21  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cloud-network-config-controller 
2023/03/24 15:00:21  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:34bfcdd0c6302a0f877a4257054302fa4f8a0549209b09d9d4c0bd8007cac9f2 
2023/03/24 15:00:21  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-authentication-operator 
2023/03/24 15:00:21  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:3a1252ab4a94ef96c90c19a926c6c10b1c73186377f408414c8a3aa1949a0a75 
2023/03/24 15:00:24  [INFO]   : completed batch 3
2023/03/24 15:00:24  [INFO]   : starting batch 4 
2023/03/24 15:00:24  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-autoscaler 
2023/03/24 15:00:24  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:debc66d73cce41b59011da70625a517f10d11342a24f69a2082b7160b7bd904e 
2023/03/24 15:00:24  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-autoscaler-operator 
2023/03/24 15:00:24  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:af36c8fe819208e55cc0346c504d641e31a0a1575420a21a6d108a67cbb978df 
2023/03/24 15:00:24  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-baremetal-operator 
2023/03/24 15:00:24  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:c4e5f13cd4d2a9556b980a8a6790c237685b007f7ea7723191bf1633d8d88e27 
2023/03/24 15:00:24  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-bootstrap 
2023/03/24 15:00:24  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:2b9577fec9c089b042a0dabe5a3b6eefb1ef0f88cf2dc6083201e5f013eba456 
2023/03/24 15:00:24  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-capi-controllers 
2023/03/24 15:00:24  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:d7047e80348f64401594a29de59ad103136d0c1db7a15194eeb892a331a84d3e 
2023/03/24 15:00:24  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-capi-operator 
2023/03/24 15:00:24  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:bbbcd1a09bf0fbf8d8d94985738cd7676b6565ed77ae2f924b4a917fd8d40786 
2023/03/24 15:00:24  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-cloud-controller-manager-operator 
2023/03/24 15:00:24  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:13397fef9671257021455712bf8242685325c97dbc6700c988bd6ab5e68ff57e 
2023/03/24 15:00:24  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-config-operator 
2023/03/24 15:00:24  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:f6ba6ec29ae317b65ccae96aae4338eed31430f09c536e09ac1e36d9f11b208e 
2023/03/24 15:00:24  [INFO]   : completed batch 4
2023/03/24 15:00:24  [INFO]   : starting batch 5 
2023/03/24 15:00:24  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-control-plane-machine-set-operator 
2023/03/24 15:00:24  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:2f4bd57474ac21367df881cb966756e1bb6941588e43622bbd2755ed7d13a9c9 
2023/03/24 15:00:24  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-csi-snapshot-controller-operator 
2023/03/24 15:00:24  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:961706a0d75013fcef5f3bbf59754ed23549316fba391249b22529d6a97f1cb2 
2023/03/24 15:00:24  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-dns-operator 
2023/03/24 15:00:24  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:da1dec5c084b77969ed1b7995a292c7ac431cdd711a708bfbe1f40628515466c 
2023/03/24 15:00:24  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-etcd-operator 
2023/03/24 15:00:24  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:a16832e9c1f864f0f8455237987cb75061483d55d4fd2619af2f93ac3563390d 
2023/03/24 15:00:24  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-image-registry-operator 
2023/03/24 15:00:24  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:a88885cb6347b4dc8d3b6f7a8716eb17a42f8d61fa39f5fccd3f8f8d38b3ae5d 
2023/03/24 15:00:24  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-ingress-operator 
2023/03/24 15:00:24  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:6275a171f6d5a523627963860415ed0e43f1728f2dd897c49412600bf64bc9c3 
2023/03/24 15:00:24  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-kube-apiserver-operator 
2023/03/24 15:00:24  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:9ed61d19216d71cc5692c22402961b0f865ed8629f5d64f1687aa47af601c018 
2023/03/24 15:00:24  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-kube-cluster-api-operator 
2023/03/24 15:00:24  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:46d40b7c04488756317c18822de0f055a688a84dabf51a789edade07dcf74283 
2023/03/24 15:00:25  [INFO]   : completed batch 5
2023/03/24 15:00:25  [INFO]   : starting batch 6 
2023/03/24 15:00:25  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-kube-controller-manager-operator 
2023/03/24 15:00:25  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:81a043e61c07b8e93c6b082aa920d61ffa69762bcc2ef1018360026d62c11b18 
2023/03/24 15:00:25  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-kube-scheduler-operator 
2023/03/24 15:00:25  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:e045fad043f28570b754619999ffb356bedee81ff842c56a32b1b13588fc1651 
2023/03/24 15:00:25  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-kube-storage-version-migrator-operator 
2023/03/24 15:00:25  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:773fe01f949872eaae7daee9bac53f06ca4d375e3f8d6207a9a3eccaa4ab9f98 
2023/03/24 15:00:25  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-machine-approver 
2023/03/24 15:00:25  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:ec53f44c080dc784adb01a4e3b8257adffaf79a6e38f683d26bf1b384d6b7156 
2023/03/24 15:00:25  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-monitoring-operator 
2023/03/24 15:00:25  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:435bd6c8ff5825fcead3f25555f77880d645e07a8f8fd35d1e3a2f433bbebe32 
2023/03/24 15:00:25  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-network-operator 
2023/03/24 15:00:25  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:5e9e309fca4e3c3fca6aeb7adaff909c8877fbc648769c3cf48ed264eb51cc5c 
2023/03/24 15:00:25  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-node-tuning-operator 
2023/03/24 15:00:25  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:946567bcda2161bc1f55a6aa236106c947c5d863225f024c8c46f19b91b71679 
2023/03/24 15:00:25  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-openshift-apiserver-operator 
2023/03/24 15:00:25  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:87666cc451e16c135276f6405cd7d0c2ce76fd5f19f02a9654c23bb9651c54b3 
2023/03/24 15:00:26  [INFO]   : completed batch 6
2023/03/24 15:00:26  [INFO]   : starting batch 7 
2023/03/24 15:00:26  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-openshift-controller-manager-operator 
2023/03/24 15:00:26  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:53c526dc7766f65b2de93215a5f609fdc2f790717c07d15ffcbf5d4ac79d002e 
2023/03/24 15:00:26  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-platform-operators-manager 
2023/03/24 15:00:26  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:fe09b8f75b721b227649c733947e031dcfd76877b5fd80706835f0e0de2097d8 
2023/03/24 15:00:26  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-policy-controller 
2023/03/24 15:00:26  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:b9cf69b5d2fd7ddcfead1a23901c0b2b4d04aebad77094f1aeb150e1ad77bb52 
2023/03/24 15:00:26  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-samples-operator 
2023/03/24 15:00:26  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:7e5cf6294e213c4dfbd16d7f5e0bd3071703a0fde2342eb09b3957eb6a2b6b3d 
2023/03/24 15:00:26  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-storage-operator 
2023/03/24 15:00:26  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:a1a9ffe4c3d12fd672271f098a10a111ab5b3d145b7e2da447ef1aaab5189c12 
2023/03/24 15:00:26  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-update-keys 
2023/03/24 15:00:26  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:27ad97c38c1658fb6f966ad2d9206ef33a4ead0e3856856b5e212f6c6179f9c2 
2023/03/24 15:00:26  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/cluster-version-operator 
2023/03/24 15:00:26  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:a4b00a7810910ea260de0d658c0d3d1a0b9ac8867e92c34573503f11de59017a 
2023/03/24 15:00:26  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/configmap-reloader 
2023/03/24 15:00:26  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c2783e21f259126efaf89772f533f65ecf90178ff0de3cab845e0af28ca5576 
2023/03/24 15:00:26  [INFO]   : completed batch 7
2023/03/24 15:00:26  [INFO]   : starting batch 8 
2023/03/24 15:00:26  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/console 
2023/03/24 15:00:26  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:45ce25aa01ce77d2d7ad8cc2914686e07e71489c8ec625bc920f14ef83b94f33 
2023/03/24 15:00:26  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/console-operator 
2023/03/24 15:00:26  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:98d4eee0edefa5f976980e36667e217efa0575e91252a99c98bb5a7e4f1bed1f 
2023/03/24 15:00:26  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/container-networking-plugins 
2023/03/24 15:00:26  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:d03a73b14daa7fe32294f62fd5ef20edf193204d6a39df05dd4342e721e7746d 
2023/03/24 15:00:26  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/coredns 
2023/03/24 15:00:26  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:dfdf833d03dac36b747951107a25ab6424eb387bb140f344d4be8d8c7f4e895f 
2023/03/24 15:00:26  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/csi-driver-manila 
2023/03/24 15:00:26  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:135115519e10a95daff127891836d8627e391b2a09ebaee81c8a51f5118d107b 
2023/03/24 15:00:26  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/csi-driver-manila-operator 
2023/03/24 15:00:26  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:aaecefa40eb4268ee5f4a0b4e8218274e52d3ea88a6e894bb1337287a69fc43f 
2023/03/24 15:00:26  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/csi-driver-nfs 
2023/03/24 15:00:26  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:1b0dd1e20bcaaf0a9529db8d12c9d9ccf22244aa84cb49b30703ffb5813b4172 
2023/03/24 15:00:26  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/csi-driver-shared-resource 
2023/03/24 15:00:26  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:614817eab19b746ab9967aa1da46f0ff5c23756dcff3bd0eef772513ad9f8e77 
2023/03/24 15:00:28  [INFO]   : completed batch 8
2023/03/24 15:00:28  [INFO]   : starting batch 9 
2023/03/24 15:00:28  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/csi-driver-shared-resource-operator 
2023/03/24 15:00:28  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:d89776062a025fd41dc10b828f8cd4c66577d66ef54d57e2a634a843e243b020 
2023/03/24 15:00:28  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/csi-driver-shared-resource-webhook 
2023/03/24 15:00:28  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:5d263154d6c3f7d75486020633bab805257bc250308840ec3f387f96c5681489 
2023/03/24 15:00:28  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/csi-external-attacher 
2023/03/24 15:00:28  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:ab4b9ad5114d8fc53ec73045e973a8be2faa0975195113e96e07fcf8ad86a7e2 
2023/03/24 15:00:28  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/csi-external-provisioner 
2023/03/24 15:00:28  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:12be6984e3a56cbc5b2a5272a872a40b242cce8a0f167993107c6de6bf776c53 
2023/03/24 15:00:28  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/csi-external-resizer 
2023/03/24 15:00:28  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:f18e0cf76a2ac840c9f66f89f1b47aced70650021e7931199659aef9cbca31e0 
2023/03/24 15:00:28  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/csi-external-snapshotter 
2023/03/24 15:00:28  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:e8262cb60ae8fbf0bf565a5f624693daa9e6f396970dcdbba5d1ca55eb525ec0 
2023/03/24 15:00:28  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/csi-livenessprobe 
2023/03/24 15:00:28  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:836d284f0c24a1d400cebc0f25e8172c28e7476879bfffe1071fb9ceb169c9ce 
2023/03/24 15:00:28  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/csi-node-driver-registrar 
2023/03/24 15:00:28  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:6bd9d4dc813a01257767fb3b16395e8aee044feec410ed8e93ee1e82daf5a744 
2023/03/24 15:00:29  [INFO]   : completed batch 9
2023/03/24 15:00:29  [INFO]   : starting batch 10 
2023/03/24 15:00:29  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/csi-snapshot-controller 
2023/03/24 15:00:29  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:631012b7d9f911558fa49e34402be56a1587a09e58ad645ce2de37aaa20eb468 
2023/03/24 15:00:29  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/csi-snapshot-validation-webhook 
2023/03/24 15:00:29  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:f876250993619037cbf206da00d0419c545269799f3b29848a9d1bc0e88aad30 
2023/03/24 15:00:29  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/deployer 
2023/03/24 15:00:29  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:f1d08a4fed20e6f9fe569f173fc039a6cc6e5f7875566e1b3a4c5cddcbf0c827 
2023/03/24 15:00:29  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/docker-builder 
2023/03/24 15:00:29  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:91496407bea46c76443337eb7fb54aa9ddf9f69856940c2d5ec1df4220d0f8fb 
2023/03/24 15:00:29  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/docker-registry 
2023/03/24 15:00:29  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:2f04a30cd7a5b862c7b8f22001aef3eaef191eb24f4c737039d7061609a2955a 
2023/03/24 15:00:29  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/driver-toolkit 
2023/03/24 15:00:29  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:b53883ca2bac5925857148c4a1abc300ced96c222498e3bc134fe7ce3a1dd404 
2023/03/24 15:00:29  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/egress-router-cni 
2023/03/24 15:00:29  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:cad5a85f21da1d2e653f41f82db607ab6827da0468283f63694c509e39374f0d 
2023/03/24 15:00:29  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/etcd 
2023/03/24 15:00:29  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:24d9b9d9d7fadacbc505c849a1e4b390b2f0fcd452ad851b7cce21e8cfec2020 
2023/03/24 15:00:32  [INFO]   : completed batch 10
2023/03/24 15:00:32  [INFO]   : starting batch 11 
2023/03/24 15:00:32  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/gcp-cloud-controller-manager 
2023/03/24 15:00:32  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:924aacef830f58390824be7fa30095562bb59f377e9547499e2eafc5edf3fbd3 
2023/03/24 15:00:32  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/gcp-cluster-api-controllers 
2023/03/24 15:00:32  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:0c1c09ad463a505d1bd15f8aadaa8741b1b472b563548d378f96378a9310d612 
2023/03/24 15:00:32  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/gcp-machine-controllers 
2023/03/24 15:00:32  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:960b368249c9d4c660e7ac8494d9c85ecc157807c691f4842757504953be45e1 
2023/03/24 15:00:32  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/gcp-pd-csi-driver 
2023/03/24 15:00:32  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:df2a341e8ff6aeedbcbeca3a1ec6d55d96b41b9398064c7fc4d0a376e714b34f 
2023/03/24 15:00:32  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/gcp-pd-csi-driver-operator 
2023/03/24 15:00:32  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:0a6f39d0633539f922b48117cea4291e76cefc654d715862a574cc9d09cabb63 
2023/03/24 15:00:32  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/haproxy-router 
2023/03/24 15:00:32  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:04cf677bb94e496d99394624e7a2334d96a87c86a3b11c5b698eb2c22ed1bcb2 
2023/03/24 15:00:32  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/hyperkube 
2023/03/24 15:00:32  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:662c00a50b8327cc39963577d3e11aa71458b3888ce06223a4501679a28fecd1 
2023/03/24 15:00:32  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/hypershift 
2023/03/24 15:00:32  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:a2d73062f7520dbcd62c44b6b816ef7f21423cadbd0b098ee756c359f26e079c 
2023/03/24 15:00:33  [INFO]   : completed batch 11
2023/03/24 15:00:33  [INFO]   : starting batch 12 
2023/03/24 15:00:33  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/ibm-cloud-controller-manager 
2023/03/24 15:00:33  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:f47dafc7ee3a7fa1f7b02ec61fa42050b359d039fef0a72e3eecaf54803bf405 
2023/03/24 15:00:33  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/ibm-vpc-block-csi-driver 
2023/03/24 15:00:33  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:6a39c87357c77e9a95115229c673c0194ac0d343ffc61fb057516c00306442e7 
2023/03/24 15:00:33  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/ibm-vpc-block-csi-driver-operator 
2023/03/24 15:00:33  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:c10a40e725a753741a94948c2e8889214650ec564a7959df85b8bbf4be2a3b03 
2023/03/24 15:00:33  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/ibm-vpc-node-label-updater 
2023/03/24 15:00:33  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:c3370a5e63eecf56ccdc17e8523a0ed836c8a5c775d1ce1bfe6b63042bba098a 
2023/03/24 15:00:33  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/ibmcloud-cluster-api-controllers 
2023/03/24 15:00:33  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:f01c2cdc020742cf5d49260a92e2bfe4984e37e0c5409f5522144299b73c7cb9 
2023/03/24 15:00:33  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/ibmcloud-machine-controllers 
2023/03/24 15:00:33  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:359cd94c60b36d288b2e60113984e465c5741ee8c461d33c0cb50777e6d5f19f 
2023/03/24 15:00:33  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/insights-operator 
2023/03/24 15:00:33  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:cca18ee8f9680c755e8b31aeec058b8d29fbdc6cc5dfb03158391fe81ee166bc 
2023/03/24 15:00:33  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/installer 
2023/03/24 15:00:33  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:1739c7cf0844140d041edce5e99654e9276ca9b2542ec00c4fd327495dbd6cf5 
2023/03/24 15:00:34  [INFO]   : completed batch 12
2023/03/24 15:00:34  [INFO]   : starting batch 13 
2023/03/24 15:00:34  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/installer-artifacts 
2023/03/24 15:00:34  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:948a67096a43e8a08556145d4ab6efb8447780c7956baf8feec1756cd2b8df3f 
2023/03/24 15:00:34  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/ironic 
2023/03/24 15:00:34  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:30328143480d6598d0b52d41a6b755bb0f4dfe04c4b7aa7aefd02ea793a2c52b 
2023/03/24 15:00:34  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/ironic-agent 
2023/03/24 15:00:34  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:920ffbcfe14f5b2122d1fe7c079fa1d45df63c0e6b50ce9c953dcc2d482bcc82 
2023/03/24 15:00:34  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/ironic-machine-os-downloader 
2023/03/24 15:00:34  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:1d705b2e2406512e5cb28239d92bb062ce3152880d74644a8f2728a27c28a6aa 
2023/03/24 15:00:34  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/ironic-static-ip-manager 
2023/03/24 15:00:34  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:4c62cd5fac0f7f8ae25b2498a02ebdc79b4af10c4246c1ac3da0b1d3a46407ec 
2023/03/24 15:00:34  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/k8s-prometheus-adapter 
2023/03/24 15:00:34  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:56e8f74cab8fdae7f7bbf1c9a307a5fb98eac750a306ec8073478f0899259609 
2023/03/24 15:00:34  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/keepalived-ipfailover 
2023/03/24 15:00:34  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:1d05f6f7f9426edfc97bfe275521d1e885883a3ba274f390b013689403727edb 
2023/03/24 15:00:34  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/kube-proxy 
2023/03/24 15:00:34  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:d6581f4956c34f0581d62185568fd42d8d321568144427e17c078f0338e34b4d 
2023/03/24 15:00:40  [INFO]   : completed batch 13
2023/03/24 15:00:40  [INFO]   : starting batch 14 
2023/03/24 15:00:40  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/kube-rbac-proxy 
2023/03/24 15:00:40  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:c28f27a3a10df13e5e8c074e8734683a6603ebaccd9d67e2095070fb6859b1d6 
2023/03/24 15:00:40  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/kube-state-metrics 
2023/03/24 15:00:40  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:f2f70f1bd12128213b7b131782a4e76df20cbc224b13c69fff7ec71787b5499e 
2023/03/24 15:00:40  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/kube-storage-version-migrator 
2023/03/24 15:00:40  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:e36c1c4e383fd252168aa2cb465236aa642062446aa3a026f06ea4a4afb52d7f 
2023/03/24 15:00:40  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/kubevirt-cloud-controller-manager 
2023/03/24 15:00:40  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:b01066c3af59b3c626f6319cab69d1e2e9d2dc6efa6601c727ee6a79d671d3ca 
2023/03/24 15:00:40  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/kubevirt-csi-driver 
2023/03/24 15:00:40  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:a9c6e29a6653384fc859a34273f34dbb38a62a06676196767700d54b1c962b68 
2023/03/24 15:00:40  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/kuryr-cni 
2023/03/24 15:00:40  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:6098321849ea5533809a4a50970d4bea844db65eaf3ec6bf4e730db8d588dda2 
2023/03/24 15:00:40  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/kuryr-controller 
2023/03/24 15:00:40  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:1d50aa23307ef22fd98ae5870fb3e8194a92db206a7203bb2bb231805858f4f2 
2023/03/24 15:00:40  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/libvirt-machine-controllers 
2023/03/24 15:00:40  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:559d764897ea8e2161fea6ef8ea19aa5f298716598659d4e158307d88cd7ef91 
2023/03/24 15:00:41  [INFO]   : completed batch 14
2023/03/24 15:00:41  [INFO]   : starting batch 15 
2023/03/24 15:00:41  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/machine-api-operator 
2023/03/24 15:00:41  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:2f103b47d3e0474680cccca26e7be28ab59e8c9b4879cdf878ea830aaf515416 
2023/03/24 15:00:41  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/machine-config-operator 
2023/03/24 15:00:41  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:e6f9b6fdba34485dfdec1d31ca0a04a85eff54174688dc402692f78f46743ef4 
2023/03/24 15:00:41  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/machine-image-customization-controller 
2023/03/24 15:00:41  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:0b64eced17c0ca3122c2f2b0992c10608cc51b19db6945f693c3f60df599052a 
2023/03/24 15:00:41  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/machine-os-content 
2023/03/24 15:00:41  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:aacc13a0c82c0a9d86f600388cdc4e60da16b8fc35959cdf9068dbeec5fce0ab 
2023/03/24 15:00:41  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/machine-os-images 
2023/03/24 15:00:41  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:d4b0cfad345fbb97c2d092268dc53cdec17c5e3115212fefcfb58c7ac4652717 
2023/03/24 15:00:41  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/multus-admission-controller 
2023/03/24 15:00:41  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:f87f071c3aa8b3932f33cd2dec201abbf7a116e70eeb0df53f93cccc0c3f4041 
2023/03/24 15:00:41  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/multus-cni 
2023/03/24 15:00:41  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:ae783ee6a05beafca04f0766933ee1573b70231a6cd8c449a2177afdaf4802a0 
2023/03/24 15:00:41  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/multus-networkpolicy 
2023/03/24 15:00:41  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:6dfb088441caa4aed126cc0ccc02e82c663f1922e4ae13864357e5c5edc6e539 
2023/03/24 15:00:48  [INFO]   : completed batch 15
2023/03/24 15:00:48  [INFO]   : starting batch 16 
2023/03/24 15:00:48  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/multus-route-override-cni 
2023/03/24 15:00:48  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:238c03849ea26995bfde9657c7628ae0e31fe35f4be068d7326b65acb1f55d01 
2023/03/24 15:00:48  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/multus-whereabouts-ipam-cni 
2023/03/24 15:00:48  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:1d74f4833b6bb911b57cc08a170a7242733bb5d09ac9480399395a1970e21365 
2023/03/24 15:00:48  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/must-gather 
2023/03/24 15:00:48  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:07d359641e4827bc34e31a5cefaf986acf4e78064fef531124cac0ed6da3e94c 
2023/03/24 15:00:48  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/network-interface-bond-cni 
2023/03/24 15:00:48  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:9c407846948c8ff2cd441089c6a57822cfe1a07a537dff1f9d7ebf2db2d1cdee 
2023/03/24 15:00:48  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/network-metrics-daemon 
2023/03/24 15:00:48  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:41709e49c29b2764beb118cc25216916be6f8db716ed51886b8191ea695d94e0 
2023/03/24 15:00:48  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/network-tools 
2023/03/24 15:00:48  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:78c082bce19429174ebd5582565c23c1ff0c7e98a73aeb62b2a4083ea292a2d3 
2023/03/24 15:00:48  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/nutanix-machine-controllers 
2023/03/24 15:00:48  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:fc1a3d5dd67b5e15339b9a5663897a02948853e4dfa8414b0d883210eacf51f6 
2023/03/24 15:00:48  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/oauth-apiserver 
2023/03/24 15:00:48  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:dfca545c1b42ae20c6465e61cf16a44f9411d9ed30af1f9017ed6da0d7ebd216 
2023/03/24 15:00:49  [INFO]   : completed batch 16
2023/03/24 15:00:49  [INFO]   : starting batch 17 
2023/03/24 15:00:49  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/oauth-proxy 
2023/03/24 15:00:49  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:f968922564c3eea1c69d6bbe529d8970784d6cae8935afaf674d9fa7c0f72ea3 
2023/03/24 15:00:49  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/oauth-server 
2023/03/24 15:00:49  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:7d756df4dce6ace35ff2aecf459affb7cc1bef2aa08004d62575ec09f6c76c86 
2023/03/24 15:00:49  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/oc-mirror 
2023/03/24 15:00:49  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:9434f74409e1b349f5b9c088b6c831089010e4ed2fa027ff3f035e773ab495fb 
2023/03/24 15:00:49  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/olm-rukpak 
2023/03/24 15:00:49  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:5be59eba6970435d4d886f5b17ba117df5d24c84150406de4f30194534df7f0d 
2023/03/24 15:00:49  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/openshift-apiserver 
2023/03/24 15:00:49  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:47bc752254f826905ac36cc2eb1819373a3045603e5dfa03c7f9e6d73c3fd9f9 
2023/03/24 15:00:49  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/openshift-controller-manager 
2023/03/24 15:00:49  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:fbeeb31a94b29354971d11e3db852e7a6ec8d2b70b8ec323a01b124281e49261 
2023/03/24 15:00:49  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/openshift-state-metrics 
2023/03/24 15:00:49  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:06285dddb5ba9bce5a5ddd07f685f1bc766abed1e0c3890621df281ddc19ab1c 
2023/03/24 15:00:49  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/openstack-cinder-csi-driver 
2023/03/24 15:00:49  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:06e9c50a2a02577e1edd074072d58cb18ebc6ab74fb07238111b28af065a12cb 
2023/03/24 15:00:50  [INFO]   : completed batch 17
2023/03/24 15:00:50  [INFO]   : starting batch 18 
2023/03/24 15:00:50  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/openstack-cinder-csi-driver-operator 
2023/03/24 15:00:50  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:58433407a13004176dd3285def578cf18e1625c46c92e408dc4605607897fd24 
2023/03/24 15:00:50  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/openstack-cloud-controller-manager 
2023/03/24 15:00:50  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:8f4e1b0fd341949e3144365f9de0fe874ac037d439f8db613f250a472da04545 
2023/03/24 15:00:50  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/openstack-machine-api-provider 
2023/03/24 15:00:50  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:1b7e4df1cb802bf56e1cb0e46131a3b1da20c0666397a87ae4dab61f2fea5c02 
2023/03/24 15:00:50  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/openstack-machine-controllers 
2023/03/24 15:00:50  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:cbf4dd5b96efc49d4d14bcdfa652ed48e9382e8c068c0bb321edef6ae94c4207 
2023/03/24 15:00:50  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/operator-lifecycle-manager 
2023/03/24 15:00:50  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:f1d71ba084c63e2d6b3140b9cbada2b50bb6589a39a526dedb466945d284c73e 
2023/03/24 15:00:50  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/operator-marketplace 
2023/03/24 15:00:50  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:a1858b601ef47c6354128261ac6435fd5bc5dc6947d9f89c02ae9d05fe5e0a10 
2023/03/24 15:00:50  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/operator-registry 
2023/03/24 15:00:50  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:46a0c17957b0dc426d81e29365727746d24784d83148457d1d846b5830d2d45d 
2023/03/24 15:00:50  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/ovirt-csi-driver 
2023/03/24 15:00:50  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:6836669f4cd027feee475175af282686fc1ad2ed8862e54726e2c897865f8d21 
2023/03/24 15:00:54  [INFO]   : completed batch 18
2023/03/24 15:00:54  [INFO]   : starting batch 19 
2023/03/24 15:00:54  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/ovirt-csi-driver-operator 
2023/03/24 15:00:54  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:925d8dc47e1602c31785a7e8c2c43f06e210c80867ceebadb052351e553c13e9 
2023/03/24 15:00:54  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/ovirt-machine-controllers 
2023/03/24 15:00:54  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:f6488d79fe2ec4081904fea31a582aad4192ba62f2dcbddfe3ef50202748a28e 
2023/03/24 15:00:54  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/ovn-kubernetes 
2023/03/24 15:00:54  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:c74fcd7470b682be673ccbc763ac25783f6997a253c8ca20f63b789520eb65bf 
2023/03/24 15:00:54  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/ovn-kubernetes-microshift 
2023/03/24 15:00:54  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:84ec1f865f595a3d0d36a7149ebcb959c6a5aa5e24333ca9d378d9496a9a91ae 
2023/03/24 15:00:54  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/pod 
2023/03/24 15:00:54  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:2037fa0130ef960eef0661e278466a67eccc1460d37f7089f021dc94dfccd52b 
2023/03/24 15:00:54  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/powervs-block-csi-driver 
2023/03/24 15:00:54  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:924e1f8672e3d6bf42997489edab26998999e960d81cd0e0491ac39d278fe48f 
2023/03/24 15:00:54  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/powervs-block-csi-driver-operator 
2023/03/24 15:00:54  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:c263b11dcd5304d779999f2540a30ea190b3a0f760a2e246c96d37a92c0f3492 
2023/03/24 15:00:54  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/powervs-cloud-controller-manager 
2023/03/24 15:00:54  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:e40f30a04d66ff1fc06d7b46f2f02da87269a1e11d79c2ab3db77d7d021cc163 
2023/03/24 15:00:56  [INFO]   : completed batch 19
2023/03/24 15:00:56  [INFO]   : starting batch 20 
2023/03/24 15:00:56  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/powervs-machine-controllers 
2023/03/24 15:00:56  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:e7da6fad74fc1437c265d512669f75f238e9da7cb8c179d43f40db30a2e8bec7 
2023/03/24 15:00:56  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/prom-label-proxy 
2023/03/24 15:00:56  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:756f3f02d7592b100d5fcf2a8351a570102e79e02425d9b5d3d09a63ee21839d 
2023/03/24 15:00:56  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/prometheus 
2023/03/24 15:00:56  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:917b84445c725430f74f2041baa697d86d2a0bc971f6b9101591524daf8053f6 
2023/03/24 15:00:56  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/prometheus-alertmanager 
2023/03/24 15:00:56  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:782acf9917df2dff59e1318fc08487830240019e5cc241e02e39a06651900bc2 
2023/03/24 15:00:56  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/prometheus-config-reloader 
2023/03/24 15:00:56  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:d1705c63614eeb3feebc11b29e6a977c28bac2401092efae1d42b655259e2629 
2023/03/24 15:00:56  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/prometheus-node-exporter 
2023/03/24 15:00:56  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:7ecf76246d81adfe3f52fdb54a7bddf6b892ea6900521d71553d16f2918a2cac 
2023/03/24 15:00:56  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/prometheus-operator 
2023/03/24 15:00:56  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:503846d640ded8b0deedc7c69647320065055d3d2a423993259692362c5d5b86 
2023/03/24 15:00:56  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/prometheus-operator-admission-webhook 
2023/03/24 15:00:56  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:30784b4b00568946c30c1830da739d61193a622cc3a17286fe91885f0c93af9f 
2023/03/24 15:00:56  [INFO]   : completed batch 20
2023/03/24 15:00:56  [INFO]   : starting batch 21 
2023/03/24 15:00:56  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/rhel-coreos-8 
2023/03/24 15:00:56  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:6db665511f305ef230a2c752d836fe073e80550dc21cede3c55cf44db01db365 
2023/03/24 15:00:56  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/rhel-coreos-8-extensions 
2023/03/24 15:00:56  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:1b967b048fac8006b51e715dfc1720ee3f3dd6dadff6baab4fd07c3ec378a6f0 
2023/03/24 15:00:56  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/route-controller-manager 
2023/03/24 15:00:56  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:7c23b71619bd88c1bfa093cfa1a72db148937e8f1637c99ff164bf566eaf78b8 
2023/03/24 15:00:56  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/sdn 
2023/03/24 15:00:56  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:87d28ea25934e84f93eea0235344d33039c5ee5cce80ca5a1a0c8bca82797f5d 
2023/03/24 15:00:56  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/service-ca-operator 
2023/03/24 15:00:56  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:14794ac4b4b5e1bb2728d253b939578a03730cf26ba5cf795c8e2d26b9737dd6 
2023/03/24 15:00:56  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/telemeter 
2023/03/24 15:00:56  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:51c76ce72315ae658d91de6620d8dd4f798e6ea0c493e5d2899dd2c52fbcd931 
2023/03/24 15:00:56  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/tests 
2023/03/24 15:00:56  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:7233826e59b2b730c43c257f81971b8f517df42ba43469af61822fdd88ebff32 
2023/03/24 15:00:56  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/thanos 
2023/03/24 15:00:56  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:00d1be95201020c5cb1d3fae3435ee9e7dc22d8360481ec8609fa368c6ad306e 
2023/03/24 15:01:04  [INFO]   : completed batch 21
2023/03/24 15:01:04  [INFO]   : starting batch 22 
2023/03/24 15:01:04  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/tools 
2023/03/24 15:01:04  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:d4edb2f3ffbd1a017d150cd1d6fca7f3cdc6a1a4e34c21a6aee0ab0366920bf0 
2023/03/24 15:01:04  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/vsphere-cloud-controller-manager 
2023/03/24 15:01:04  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:a8e933351f3a010239ecfe1bdc74a8e2502b29fd7b7c05fcccfc2d48e980ea2c 
2023/03/24 15:01:04  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/vsphere-cluster-api-controllers 
2023/03/24 15:01:04  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:fc5cd7d5ac1dcb4049a819e3e557ec78991bc01fd2d5fee252c284a37c0ec631 
2023/03/24 15:01:04  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/vsphere-csi-driver 
2023/03/24 15:01:04  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:e0c13c6d2ab5436bf18211b15bf2ca9d3798964f5e03ed761c0e4708cf8d4e88 
2023/03/24 15:01:04  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/vsphere-csi-driver-operator 
2023/03/24 15:01:04  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:1ddea003dd0de85bb4f6d7e9106c54125858a162fe1fda1f238258418fcb52e8 
2023/03/24 15:01:04  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/vsphere-csi-driver-syncer 
2023/03/24 15:01:04  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:89cea54cc62014f0c2709b27942fdae5f7613e8f1540ea51b44328cf166b987f 
2023/03/24 15:01:04  [DEBUG]  : source dir:working-dir/test-lmz/release-images/ocp-release/4.12.0-x86_64/images/vsphere-problem-detector 
2023/03/24 15:01:04  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/openshift-release-dev/ocp-v4.0-art-dev@sha256:80a6200b0c18486bad5abe5804a62bed7436daa56f0296e7b19c18f84b3b8b1b 
2023/03/24 15:01:04  [DEBUG]  : source oci:working-dir/test-lmz/operator-images/redhat-operator-index/v4.12/aws-load-balancer-operator.v0.2.0/albo/aws-load-balancer-rhel8-operator-ab38b37c14f7f0897e09a18eca4a232a6c102b76e9283e401baed832852290b5-annotation 
2023/03/24 15:01:04  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/working-dir/test-lmz/redhat-operator-index/v4.12/aws-load-balancer-operator.v0.2.0/albo 
2023/03/24 15:01:05  [INFO]   : completed batch 22
2023/03/24 15:01:05  [INFO]   : executing remainder [batch size of 1]
2023/03/24 15:01:05  [INFO]   : images to mirror 5 
2023/03/24 15:01:05  [INFO]   : batch count 1 
2023/03/24 15:01:05  [INFO]   : batch index 0 
2023/03/24 15:01:05  [INFO]   : batch size 5 
2023/03/24 15:01:05  [INFO]   : remainder size 0 
2023/03/24 15:01:05  [INFO]   : starting batch 0 
2023/03/24 15:01:05  [DEBUG]  : source oci:working-dir/test-lmz/operator-images/redhat-operator-index/v4.12/aws-load-balancer-operator.v0.2.0/albo/cb36fc 
2023/03/24 15:01:05  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/working-dir/test-lmz/redhat-operator-index/v4.12/aws-load-balancer-operator.v0.2.0/albo 
2023/03/24 15:01:05  [DEBUG]  : source oci:working-dir/test-lmz/operator-images/redhat-operator-index/v4.12/aws-load-balancer-operator.v0.2.0/albo/controller 
2023/03/24 15:01:05  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/working-dir/test-lmz/redhat-operator-index/v4.12/aws-load-balancer-operator.v0.2.0/albo 
2023/03/24 15:01:05  [DEBUG]  : source oci:working-dir/test-lmz/operator-images/redhat-operator-index/v4.12/aws-load-balancer-operator.v0.2.0/albo/manager 
2023/03/24 15:01:05  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/working-dir/test-lmz/redhat-operator-index/v4.12/aws-load-balancer-operator.v0.2.0/albo 
2023/03/24 15:01:05  [DEBUG]  : source oci:working-dir/test-lmz/operator-images/redhat-operator-index/v4.12/aws-load-balancer-operator.v0.2.0/openshift4/kube-rbac-proxy 
2023/03/24 15:01:05  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/working-dir/test-lmz/redhat-operator-index/v4.12/aws-load-balancer-operator.v0.2.0/openshift4 
2023/03/24 15:01:05  [DEBUG]  : source oci:working-dir/test-lmz/additional-images/ubi9/ubi 
2023/03/24 15:01:05  [DEBUG]  : destination docker://localhost.localdomain:5000/testlmz/working-dir/test-lmz/ubi9 
2023/03/24 15:01:05  [INFO]   : completed batch 0


```



