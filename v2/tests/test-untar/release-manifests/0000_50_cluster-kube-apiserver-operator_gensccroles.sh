#!/bin/bash

read -r -d '' role_template <<"EOF"
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  annotations:
    include.release.openshift.io/ibm-cloud-managed: "true"
    include.release.openshift.io/self-managed-high-availability: "true"
    include.release.openshift.io/single-node-developer: "true"
    rbac.authorization.kubernetes.io/autoupdate: "true"
  name: system:openshift:scc:<SCC_NAME>
rules:
- apiGroups:
  - security.openshift.io
  resourceNames:
  - <SCC_NAME>
  resources:
  - securitycontextconstraints
  verbs:
  - use
EOF

SCC_FILE_PREFIX="0000_20_kube-apiserver-operator_00_scc-"
SCC_ROLE_FILE_PREFIX="0000_20_kube-apiserver-operator_00_cr-scc-"

scc_names=$(ls -1 | grep "$SCC_FILE_PREFIX" | sed -e "s/$SCC_FILE_PREFIX//g" | sed -e 's/\.yaml//g')

for name in ${scc_names[@]}; do
    echo "$role_template" | sed "s/<SCC_NAME>/$name/g" > "${SCC_ROLE_FILE_PREFIX}${name}.yaml"
done
