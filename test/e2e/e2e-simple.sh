#!/usr/bin/env bash

set -eu

DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
source "$DIR/lib/check.sh"
source "$DIR/lib/workflow.sh"
source "$DIR/lib/util.sh"
source "$DIR/testcases.sh"

CMD="${1:?cmd bin path is required}"
CMD="$(cd "$(dirname "$CMD")" && pwd)/$(basename "$CMD")"

DATA_TMP=$(mktemp -d "${DIR}/operator-test.XXXXX")
CREATE_FULL_DIR="${DATA_TMP}/create_full"
CREATE_DIFF_DIR="${DATA_TMP}/create_diff"
PUBLISH_FULL_DIR="${DATA_TMP}/publish_full"
PUBLISH_DIFF_DIR="${DATA_TMP}/publish_diff"
REGISTRY_CONN_DIR="${DATA_TMP}/conn"
REGISTRY_DISCONN_DIR="${DATA_TMP}/disconn"
MIRROR_OCI_DIR="${DATA_TMP}/mirror_oci"
WORKSPACE="oc-mirror-workspace"
CATALOGREGISTRY="quay.io"
CATALOGORG="redhatgov"
CATALOGNAMESPACE="redhatgov/oc-mirror-dev"
REGISTRY_CONN_PORT=5000
REGISTRY_DISCONN_PORT=5001
METADATA_REGISTRY="localhost.localdomain:$REGISTRY_CONN_PORT"
METADATA_CATALOGNAMESPACE="${METADATA_REGISTRY}/${CATALOGNAMESPACE}"
METADATA_OCI_CATALOG="oci://${DIR}/artifacts/rhop-ctlg-oci"
TARGET_CATALOG_NAME="target-name"
TARGET_CATALOG_TAG="target-tag"


GOBIN=$HOME/go/bin
PATH=$PATH:$GOBIN

trap cleanup_all EXIT

# Install crane and registry2
install_deps

echo "INFO: Running ${#TESTCASES[@]} test cases"

for i in "${!TESTCASES[@]}"; do
    echo "INFO: Running ${TESTCASES[$i]}"
    mkdir -p "$DATA_TMP"
    setup_reg 
    ${TESTCASES[$i]}
    rm -rf "$DATA_TMP"
    cleanup_all
done
