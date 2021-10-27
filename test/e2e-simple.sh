#!/usr/bin/env bash

set -eu

source test/lib.sh

CMD="${1:?cmd bin path is required}"

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DATA_TMP=$(mktemp -d "${DIR}/operator-test.XXXXX")
CREATE_FULL_DIR="${DATA_TMP}/create_full"
CREATE_DIFF_DIR="${DATA_TMP}/create_diff"
PUBLISH_FULL_DIR="${DATA_TMP}/publish_full"
PUBLISH_DIFF_DIR="${DATA_TMP}/publish_diff"
REGISTRY_CONN="localhost"
REGISTRY_CONN_PORT=5000
REGISTRY_DISCONN="localhost"
REGISTRY_DISCONN_PORT=5000

#trap "${DIR}/stop-docker-registry.sh $REGISTRY_CONN; ${DIR}/stop-docker-registry.sh $REGISTRY_DISCONN" EXIT

## Test `create full`

# Test full catalog mode.
#"${DIR}/start-docker-registry.sh" $REGISTRY_CONN $REGISTRY_CONN_PORT
"${DIR}/operator/setup-testdata.sh" "$DATA_TMP" "$CREATE_FULL_DIR" "latest/imageset-config-full.yaml" false
run_cmd create full --dir "$CREATE_FULL_DIR" --config "${CREATE_FULL_DIR}/imageset-config-full.yaml" --output "$DATA_TMP"
# Stop the connected registry so we're sure nothing is being pulled from it.
#"${DIR}/stop-docker-registry.sh" $REGISTRY_CONN
#"${DIR}/start-docker-registry.sh" $REGISTRY_DISCONN $REGISTRY_DISCONN_PORT
run_cmd publish --dir "$PUBLISH_FULL_DIR" --archive "${DATA_TMP}/bundle_seq1_000000.tar" --to-mirror $REGISTRY_DISCONN:$REGISTRY_DISCONN_PORT
check_bundles ${REGISTRY_DISCONN}:${REGISTRY_DISCONN_PORT}/test-catalogs/test-catalog:latest \
  "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.0.0 baz.v1.0.1 baz.v1.1.0 foo.v0.1.0 foo.v0.2.0 foo.v0.3.0 foo.v0.3.1" \
  ${REGISTRY_DISCONN}:${REGISTRY_DISCONN_PORT}
#"${DIR}/stop-docker-registry.sh" $REGISTRY_DISCONN
rm -rf "$DATA_TMP"

# Test heads-only catalog mode.
mkdir "$DATA_TMP"
#"${DIR}/start-docker-registry.sh" $REGISTRY_CONN $REGISTRY_CONN_PORT
"${DIR}/operator/setup-testdata.sh" "$DATA_TMP" "$CREATE_FULL_DIR" "latest/imageset-config-headsonly.yaml" false
run_cmd create full --dir "$CREATE_FULL_DIR" --config "${CREATE_FULL_DIR}/imageset-config-headsonly.yaml" --output "$DATA_TMP"
#"${DIR}/start-docker-registry.sh" $REGISTRY_DISCONN $REGISTRY_DISCONN_PORT
run_cmd publish --dir "$PUBLISH_FULL_DIR" --archive "${DATA_TMP}/bundle_seq1_000000.tar" --to-mirror $REGISTRY_DISCONN:$REGISTRY_DISCONN_PORT
check_bundles ${REGISTRY_DISCONN}:${REGISTRY_DISCONN_PORT}/test-catalogs/test-catalog:latest \
  "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
  ${REGISTRY_DISCONN}:${REGISTRY_DISCONN_PORT}

#test `create diff` with new operator bundles and releases.
mkdir -p "${CREATE_DIFF_DIR}/src/publish"
mkdir -p "${PUBLISH_DIFF_DIR}/publish"
cp "${CREATE_FULL_DIR}/src/publish/.metadata.json" "${CREATE_DIFF_DIR}/src/publish/"
cp "${PUBLISH_FULL_DIR}/publish/.metadata.json" "${PUBLISH_DIFF_DIR}/publish/"
cp "${CREATE_FULL_DIR}/imageset-config-headsonly.yaml" ${CREATE_DIFF_DIR}
"${DIR}/operator/setup-testdata.sh" "$DATA_TMP" "$CREATE_DIFF_DIR" "latest/imageset-config-headsonly.yaml" true
run_cmd create diff --dir "$CREATE_DIFF_DIR" --config "${CREATE_DIFF_DIR}/imageset-config-headsonly.yaml" --output "$DATA_TMP"
#"${DIR}/stop-docker-registry.sh" $REGISTRY_CONN
run_cmd publish --dir "$PUBLISH_DIFF_DIR" --archive "${DATA_TMP}/bundle_seq2_000000.tar" --to-mirror $REGISTRY_DISCONN:$REGISTRY_DISCONN_PORT
check_bundles ${REGISTRY_DISCONN}:${REGISTRY_DISCONN_PORT}/test-catalogs/test-catalog:latest \
"bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1 foo.v0.3.2" \
 ${REGISTRY_DISCONN}:${REGISTRY_DISCONN_PORT}
#"${DIR}/stop-docker-registry.sh" $REGISTRY_DISCONN
rm -rf "$DATA_TMP"