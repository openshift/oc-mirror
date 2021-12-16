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

# Test full catalog mode.
setup_reg
run_full imageset-config-full.yaml false
check_bundles localhost:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
"bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.0.0 baz.v1.0.1 baz.v1.1.0 foo.v0.1.0 foo.v0.2.0 foo.v0.3.0 foo.v0.3.1" \
localhost:${REGISTRY_DISCONN_PORT}
rm -rf "$DATA_TMP"
cleanup

# Test heads-only mode
mkdir "$DATA_TMP"
setup_reg
run_full imageset-config-headsonly.yaml true
check_bundles localhost:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
"bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
localhost:${REGISTRY_DISCONN_PORT}

# Test headsonly diff
run_diff imageset-config-headsonly.yaml
check_bundles localhost:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
"bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1 foo.v0.3.2" \
localhost:${REGISTRY_DISCONN_PORT}
rm -rf "$DATA_TMP"
cleanup

# Test registry backend
mkdir "$DATA_TMP"
setup_reg
run_full imageset-config-headsonly-backend-registry.yaml true
check_bundles localhost:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
"bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
localhost:${REGISTRY_DISCONN_PORT}

# Test regsitry backend diff
run_diff imageset-config-headsonly-backend-registry.yaml
check_bundles localhost:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
"bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1 foo.v0.3.2" \
localhost:${REGISTRY_DISCONN_PORT}
rm -rf "$DATA_TMP"
cleanup

# Test mirror to mirror
mkdir "$DATA_TMP"
setup_reg
mirror2mirror imageset-config-headsonly.yaml
check_bundles localhost:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
"bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
localhost:${REGISTRY_DISCONN_PORT}
rm -rf "$DATA_TMP"
cleanup

# Test mirror to mirror no backend
mkdir "$DATA_TMP"
setup_reg
mirror2mirror imageset-config-full.yaml
check_bundles localhost:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
"bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.0.0 baz.v1.0.1 baz.v1.1.0 foo.v0.1.0 foo.v0.2.0 foo.v0.3.0 foo.v0.3.1" \
localhost:${REGISTRY_DISCONN_PORT}
rm -rf "$DATA_TMP"
cleanup

# Test registry backend with custom namespace
mkdir "$DATA_TMP"
setup_reg
run_full imageset-config-headsonly-backend-registry.yaml true "custom"
check_bundles "localhost:${REGISTRY_DISCONN_PORT}/custom/${CATALOGNAMESPACE}:test-catalog-latest" \
"bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
localhost:${REGISTRY_DISCONN_PORT} "custom"

# Test registry backend with custom namespace diff
run_diff imageset-config-headsonly-backend-registry.yaml "custom"
check_bundles "localhost:${REGISTRY_DISCONN_PORT}/custom/${CATALOGNAMESPACE}:test-catalog-latest" \
"bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1 foo.v0.3.2" \
localhost:${REGISTRY_DISCONN_PORT} "custom"
rm -rf "$DATA_TMP"
cleanup

# Test heads only while skipping deps
mkdir "$DATA_TMP"
setup_reg
run_full imageset-config-skip-deps.yaml true "custom"
check_bundles "localhost:${REGISTRY_DISCONN_PORT}/custom/${CATALOGNAMESPACE}:test-catalog-latest" \
"bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
localhost:${REGISTRY_DISCONN_PORT} "custom"