#!/usr/bin/env bash

# These are used to define all testcase
# run during end to end test
declare -a TESTCASES
TESTCASES[1]="full_catalog"
TESTCASES[2]="headsonly_diff"
TESTCASES[3]="pruned_catalogs"
TESTCASES[4]="registry_backend"
TESTCASES[5]="mirror_to_mirror"
TESTCASES[6]="mirror_to_mirror_nostorage"
TESTCASES[7]="custom_namespace"
TESTCASES[8]="package_filtering"
TESTCASES[9]="skip_deps"
TESTCASES[10]="helm_local"
TESTCASES[11]="no_updates_exist"

# Test full catalog mode.
function full_catalog () {
    workflow_full imageset-config-full.yaml "test-catalog-latest" -c="--source-use-http"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.0.0 baz.v1.0.1 baz.v1.1.0 foo.v0.1.0 foo.v0.2.0 foo.v0.3.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test heads-only mode
function headsonly_diff () {
    workflow_full imageset-config-headsonly.yaml "test-catalog-latest" --diff -c="--source-use-http"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}

    workflow_diff imageset-config-headsonly.yaml "test-catalog-diff" -c="--source-use-http"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.0 foo.v0.3.1 foo.v0.3.2" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test heads-only mode with catalogs that prune bundles
function pruned_catalogs() {
    workflow_full imageset-config-headsonly.yaml "test-catalog-prune" --diff -c="--source-use-http"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 foo.v0.1.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}

    workflow_diff imageset-config-headsonly.yaml "test-catalog-prune-diff" -c="--source-use-http"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 foo.v0.1.1 foo.v0.2.0" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test registry backend
function registry_backend () {
    workflow_full imageset-config-headsonly-backend-registry.yaml "test-catalog-latest" --diff -c="--source-use-http"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}

    workflow_diff imageset-config-headsonly-backend-registry.yaml "test-catalog-diff" -c="--source-use-http"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.0 foo.v0.3.1 foo.v0.3.2" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test mirror to mirror with local backend
function mirror_to_mirror() {
    workflow_mirror2mirror imageset-config-headsonly.yaml "test-catalog-latest" -c="--source-use-http"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test mirror to mirror no backend
function mirror_to_mirror_nostorage() {
    workflow_mirror2mirror imageset-config-full.yaml "test-catalog-latest" -c="--source-use-http"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.0.0 baz.v1.0.1 baz.v1.1.0 foo.v0.1.0 foo.v0.2.0 foo.v0.3.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test registry backend with custom namespace
function custom_namespace {
    workflow_full imageset-config-headsonly-backend-registry.yaml "test-catalog-latest" --diff -n="custom" -c="--source-use-http"
    check_bundles "localhost.localdomain:${REGISTRY_DISCONN_PORT}/custom/${CATALOGNAMESPACE}:test-catalog-latest" \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT} "custom"

    workflow_diff imageset-config-headsonly-backend-registry.yaml "test-catalog-diff" -n="custom" -c="--source-use-http"
    check_bundles "localhost.localdomain:${REGISTRY_DISCONN_PORT}/custom/${CATALOGNAMESPACE}:test-catalog-latest" \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.0 foo.v0.3.1 foo.v0.3.2" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT} "custom"
}


# Test package filtering
function package_filtering {
    workflow_full imageset-config-filter.yaml "test-catalog-latest" --diff -c="--source-use-http"
    check_bundles "localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest" \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 foo.v0.1.0 foo.v0.2.0 foo.v0.3.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}

    workflow_diff imageset-config-filter-multi.yaml "test-catalog-diff" -c="--source-use-http"
    check_bundles "localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest" \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.0.1 baz.v1.1.0 foo.v0.1.0 foo.v0.2.0 foo.v0.3.0 foo.v0.3.1 foo.v0.3.2" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test skip deps
function skip_deps {
    workflow_full imageset-config-skip-deps.yaml "test-catalog-latest" --diff -c="--source-use-http"
    check_bundles "localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest" \
    "bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}


# Test local helm chart
function helm_local {
    workflow_helm imageset-config-helm.yaml podinfo-6.0.0.tgz
    check_helm "localhost.localdomain:${REGISTRY_DISCONN_PORT}/stefanprodan/podinfo:6.0.0"
}

# Test no udpates
function no_updates_exist {
    workflow_no_updates imageset-config-headsonly.yaml "test-catalog-latest" --diff -c="--source-use-http"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}

    if [ -f ${CREATE_FULL_DIR}/mirror_seq2_000000.tar ]; then
        echo "no updates should not have a second sequence"
        exit 1
    fi
    check_sequence_number 1
}
