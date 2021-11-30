#!/usr/bin/env bash

set -eu

source test/lib.sh

CMD="${1:?cmd bin path is required}"
CMD="$(cd "$(dirname "$CMD")" && pwd)/$(basename "$CMD")"

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DATA_TMP=$(mktemp -d "${DIR}/operator-test.XXXXX")
CREATE_FULL_DIR="${DATA_TMP}/create_full"
CREATE_DIFF_DIR="${DATA_TMP}/create_diff"
PUBLISH_FULL_DIR="${DATA_TMP}/publish_full"
PUBLISH_DIFF_DIR="${DATA_TMP}/publish_diff"
WORKSPACE="oc-mirror-workspace"
REGISTRY_CONN=conn_registry
REGISTRY_CONN_PORT=5000
REGISTRY_DISCONN=disconn_registry
REGISTRY_DISCONN_PORT=5001

trap "${DIR}/stop-docker-registry.sh $REGISTRY_CONN; ${DIR}/stop-docker-registry.sh $REGISTRY_DISCONN" EXIT

function run_full() {
  local config="${1:?config required}"
  local diff="${2:?diff required}"
  mkdir $PUBLISH_FULL_DIR
  "${DIR}/start-docker-registry.sh" $REGISTRY_CONN $REGISTRY_CONN_PORT
  "${DIR}/operator/setup-testdata.sh" "$DATA_TMP" "$CREATE_FULL_DIR" "latest/$config" false
  run_cmd --config "${CREATE_FULL_DIR}/$config" "file://${CREATE_FULL_DIR}"
  # Stop the connected registry so we're sure nothing is being pulled from it.
  if [[ $diff == "false" ]]; then
    "${DIR}/stop-docker-registry.sh" $REGISTRY_CONN
  fi
  "${DIR}/start-docker-registry.sh" $REGISTRY_DISCONN $REGISTRY_DISCONN_PORT
  cd $PUBLISH_FULL_DIR
  run_cmd --from "${CREATE_FULL_DIR}/mirror_seq1_000000.tar" "docker://localhost:$REGISTRY_DISCONN_PORT"
  cd -
}

function run_diff() {
  local config="${1:?config required}"
    mkdir $PUBLISH_DIFF_DIR
  "${DIR}/operator/setup-testdata.sh" "$DATA_TMP" "$CREATE_DIFF_DIR" "latest/$config" true
  run_cmd --config "${CREATE_DIFF_DIR}/$config" "file://${CREATE_DIFF_DIR}"
  "${DIR}/stop-docker-registry.sh" $REGISTRY_CONN
  cd ${PUBLISH_DIFF_DIR}
  run_cmd --from "${CREATE_DIFF_DIR}/mirror_seq2_000000.tar" "docker://localhost:$REGISTRY_DISCONN_PORT"
  cd -
}

function mirror2mirror() {
  local config="${1:?config required}"
  mkdir $PUBLISH_FULL_DIR
  "${DIR}/start-docker-registry.sh" $REGISTRY_CONN $REGISTRY_CONN_PORT
  "${DIR}/start-docker-registry.sh" $REGISTRY_DISCONN $REGISTRY_DISCONN_PORT
  "${DIR}/operator/setup-testdata.sh" "$DATA_TMP" "$CREATE_FULL_DIR" "latest/$config" false
  cd ${CREATE_FULL_DIR}
  run_cmd --config "${CREATE_FULL_DIR}/$config" "docker://localhost:$REGISTRY_DISCONN_PORT"
  cd -
  "${DIR}/stop-docker-registry.sh" $REGISTRY_CONN
}

# Test full catalog mode.
run_full imageset-config-full.yaml false
check_bundles localhost:${REGISTRY_DISCONN_PORT}/test-catalogs/test-catalog:latest \
"bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.0.0 baz.v1.0.1 baz.v1.1.0 foo.v0.1.0 foo.v0.2.0 foo.v0.3.0 foo.v0.3.1" \
localhost:${REGISTRY_DISCONN_PORT}
"${DIR}/stop-docker-registry.sh" $REGISTRY_DISCONN
rm -rf "$DATA_TMP"

# Test heads-only catalog mode.
mkdir "$DATA_TMP"
run_full imageset-config-headsonly.yaml true
check_bundles localhost:${REGISTRY_DISCONN_PORT}/test-catalogs/test-catalog:latest \
"bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
localhost:${REGISTRY_DISCONN_PORT}

# Test `create diff` with new operator bundles and releases.
run_diff imageset-config-headsonly.yaml
check_bundles localhost:${REGISTRY_DISCONN_PORT}/test-catalogs/test-catalog:latest \
"bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1 foo.v0.3.2" \
localhost:${REGISTRY_DISCONN_PORT}
"${DIR}/stop-docker-registry.sh" $REGISTRY_DISCONN
rm -rf "$DATA_TMP"

# Test registry backend
mkdir "$DATA_TMP"
run_full imageset-config-headsonly-backend-registry.yaml true
check_bundles localhost:${REGISTRY_DISCONN_PORT}/test-catalogs/test-catalog:latest \
"bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
localhost:${REGISTRY_DISCONN_PORT}

# Test `create diff` with new operator bundles and releases.
run_diff imageset-config-headsonly-backend-registry.yaml
check_bundles localhost:${REGISTRY_DISCONN_PORT}/test-catalogs/test-catalog:latest \
"bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1 foo.v0.3.2" \
localhost:${REGISTRY_DISCONN_PORT}
"${DIR}/stop-docker-registry.sh" $REGISTRY_DISCONN
rm -rf "$DATA_TMP"

# Test mirror to mirror
mkdir "$DATA_TMP"
mirror2mirror imageset-config-headsonly.yaml
check_bundles localhost:${REGISTRY_DISCONN_PORT}/test-catalogs/test-catalog:latest \
"bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
localhost:${REGISTRY_DISCONN_PORT}
"${DIR}/stop-docker-registry.sh" $REGISTRY_DISCONN
rm -rf "$DATA_TMP"

# Test mirror to mirror no backend
mkdir "$DATA_TMP"
mirror2mirror imageset-config-full.yaml
check_bundles localhost:${REGISTRY_DISCONN_PORT}/test-catalogs/test-catalog:latest \
"bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.0.0 baz.v1.0.1 baz.v1.1.0 foo.v0.1.0 foo.v0.2.0 foo.v0.3.0 foo.v0.3.1" \
localhost:${REGISTRY_DISCONN_PORT}
"${DIR}/stop-docker-registry.sh" $REGISTRY_DISCONN
rm -rf "$DATA_TMP"