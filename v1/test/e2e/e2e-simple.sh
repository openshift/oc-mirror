#!/usr/bin/env bash

set -eu

DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
source "$DIR/lib/check.sh"
source "$DIR/lib/workflow.sh"
source "$DIR/lib/util.sh"
source "$DIR/testcases.sh"

TEST_E2E=true
export TEST_E2E

CMD="${1:?cmd bin path is required}"
CMD="$(cd "$(dirname "$CMD")" && pwd)/$(basename "$CMD")"

DATA_TMP="${DIR}/operator-test.${RANDOM}"
CREATE_FULL_DIR="${DATA_TMP}/create_full"
CREATE_DIFF_DIR="${DATA_TMP}/create_diff"
PUBLISH_FULL_DIR="${DATA_TMP}/publish_full"
PUBLISH_DIFF_DIR="${DATA_TMP}/publish_diff"
REGISTRY_CONN_DIR="${DATA_TMP}/conn"
REGISTRY_DISCONN_DIR="${DATA_TMP}/disconn"
MIRROR_OCI_DIR="${DATA_TMP}/mirror_oci"

# Enables overriding for specific architecture payloads (arm64,ppc64le,s390x,x86_64)
CATALOGORG="${ENV_CATALOGORG:-skhoury}"
CATALOGNAMESPACE="${ENV_CATALOGNAMESPACE:-skhoury/oc-mirror-dev}"
# The default is the amd64 catalog, otherwise. Architecture specific tests can pass in the correct value.
CATALOG_ID="${ENV_CATALOG_ID:-86fa1b12}"
OCI_REGISTRY_NAMESPACE="${ENV_OCI_REGISTRY_NAMESPACE:-redhatgov}"
# CATALOG_ARCH is used in subsequent tests to check architecture specific images.
CATALOG_ARCH="$(arch | sed 's|aarch64|arm64|g')"
if [ "${CATALOG_ARCH}" == "x86_64" ]
then
    OCI_CTLG="oc-mirror-dev"
    OCI_CTLG_PATH="oc-mirror-dev.tgz"
else
    OCI_CTLG="oc-mirror-${CATALOG_ARCH}-dev"
    OCI_CTLG_PATH="oc-mirror-${CATALOG_ARCH}-dev.tgz"
fi

WORKSPACE="oc-mirror-workspace"
CATALOGREGISTRY="quay.io"
REGISTRY_CONN_PORT=5000
REGISTRY_DISCONN_PORT=5001
METADATA_REGISTRY="localhost.localdomain:$REGISTRY_CONN_PORT"
METADATA_CATALOGNAMESPACE="${METADATA_REGISTRY}/${CATALOGNAMESPACE}"
METADATA_OCI_CATALOG="oci://${DIR}/artifacts/rhop-ctlg-oci"
TARGET_CATALOG_NAME="target-name"
TARGET_CATALOG_TAG="target-tag"

GOBIN=$HOME/go/bin
# Check if this is running with a different location
if [ ! -d $GOBIN ]
then 
    GOBIN=/usr/local/go/bin/
fi
PATH=$PATH:$GOBIN

mkdir -p $DATA_TMP

trap cleanup_all EXIT

# Install crane and registry2
install_deps

echo "INFO: Running ${#TESTCASES[@]} test cases"

for i in "${!TESTCASES[@]}"; do
    echo -e "INFO: Running ${TESTCASES[$i]}\n"
    mkdir -p "$DATA_TMP"
    setup_reg 
    ${TESTCASES[$i]}
    rm -rf "$DATA_TMP"
    cleanup_all
done
