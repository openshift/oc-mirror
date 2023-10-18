#!/usr/bin/env bash

set -eu

source test/lib.sh
source test/testcases.sh

export GO_COMPLIANCE_INFO=0
export GO_COMPLIANCE_DEBUG=0

CMD="${1:?cmd bin path is required}"
CMD="$(cd "$(dirname "$CMD")" && pwd)/$(basename "$CMD")"

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DATA_TMP=$(mktemp -d "${DIR}/operator-test.XXXXX")
CREATE_FULL_DIR="${DATA_TMP}/create_full"
CREATE_DIFF_DIR="${DATA_TMP}/create_diff"
PUBLISH_FULL_DIR="${DATA_TMP}/publish_full"
PUBLISH_DIFF_DIR="${DATA_TMP}/publish_diff"
REGISTRY_CONN_DIR="${DATA_TMP}/conn"
REGISTRY_DISCONN_DIR="${DATA_TMP}/disconn"
WORKSPACE="oc-mirror-workspace"
CATALOGNAMESPACE="redhatgov/oc-mirror-dev"
REGISTRY_CONN_PORT=5000
REGISTRY_DISCONN_PORT=5001
NS=""

GOBIN=$HOME/go/bin
PATH=$PATH:$GOBIN

trap cleanup EXIT

# Install crane and registry2
install_deps

for i in "${!TESTCASES[@]}"; do
    echo "INFO: Running ${TESTCASES[$i]}"
    mkdir -p "$DATA_TMP"
    setup_reg
    ${TESTCASES[$i]}
    rm -rf "$DATA_TMP"
    cleanup
done
