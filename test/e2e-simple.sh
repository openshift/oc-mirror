#!/usr/bin/env bash

set -eu

CMD="${1:?cmd bin path is required}"

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DATA_TMP=$(mktemp -d "${DIR}/operator-test.XXXXX")
CREATE_FULL_DIR="${DATA_TMP}/create_full"
CREATE_DIFF_DIR="${DATA_TMP}/create_diff"
PUBLISH_FULL_DIR="${DATA_TMP}/publish_full"
PUBLISH_DIFF_DIR="${DATA_TMP}/publish_diff"
REGISTRY_CONN=conn_registry
REGISTRY_CONN_PORT=5000
REGISTRY_DISCONN=disconn_registry
REGISTRY_DISCONN_PORT=5001

function run_cmd() {
  local test_flags="--log-level debug --skip-tls --skip-cleanup"

  echo "$CMD" $@ $test_flags
  echo
  "$CMD" $@ $test_flags
}

trap "${DIR}/stop-docker-registry.sh $REGISTRY_CONN; ${DIR}/stop-docker-registry.sh $REGISTRY_DISCONN" EXIT

"${DIR}/start-docker-registry.sh" $REGISTRY_CONN $REGISTRY_CONN_PORT
"${DIR}/start-docker-registry.sh" $REGISTRY_DISCONN $REGISTRY_DISCONN_PORT
"${DIR}/operator/setup-testdata.sh" "$DATA_TMP" "$CREATE_FULL_DIR"

run_cmd create full --dir "$CREATE_FULL_DIR" --config "${CREATE_FULL_DIR}/imageset-config.yaml" --output "$DATA_TMP"
run_cmd publish --dir "$PUBLISH_FULL_DIR" --archive "${DATA_TMP}/bundle_000000.tar.gz" --to-mirror localhost:$REGISTRY_DISCONN_PORT

# TODO: test `create diff` with new operator bundles and releases.
# rm "${DATA_TMP}/bundle_000000.tar.gz"
# mkdir -p "${CREATE_DIFF_DIR}/src/publish"
# cp "${CREATE_FULL_DIR}/src/publish/.metadata.json" "${CREATE_DIFF_DIR}/src/publish/"
# run_cmd create diff --dir "$CREATE_DIFF_DIR" --config "${CREATE_DIFF_DIR}/imageset-config.yaml" --output "$DATA_TMP"
# run_cmd publish --dir "$PUBLISH_DIFF_DIR" --archive "${DATA_TMP}/bundle_000000.tar.gz" --to-mirror localhost:$REGISTRY_DISCONN_PORT

# Clean up successful tests.
rm -rf $DATA_TMP
