#!/usr/bin/env bash

# These are used to define all testcase
# run during end to end test
declare -a TESTCASES
TESTCASES[1]="full_catalog"
TESTCASES[2]="full_catalog_with_digest"
TESTCASES[3]="headsonly_diff"
TESTCASES[4]="pruned_catalogs"
TESTCASES[5]="registry_backend"
TESTCASES[6]="mirror_to_mirror"
TESTCASES[7]="mirror_to_mirror_nostorage"
TESTCASES[8]="custom_namespace"
TESTCASES[9]="package_filtering"
TESTCASES[10]="skip_deps"
TESTCASES[11]="helm_local"
TESTCASES[12]="no_updates_exist"

# Test full catalog mode.
function full_catalog () {
    workflow_full imageset-config-full.yaml "test-catalog-latest" -c="--source-use-http"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.0.0 baz.v1.0.1 baz.v1.1.0 foo.v0.1.0 foo.v0.2.0 foo.v0.3.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test full catalog mode with digest.
function full_catalog_with_digest() {
    workflow_full imageset-config-full-digest.yaml "test-catalog-latest" -c="--source-use-http"
    TMPTAG=$(echo $CATALOGDIGEST | cut -d: -f 2)
    TMPTAG=${TMPTAG:0:6}
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:${TMPTAG}\
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
    check_image_exists "localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:535b8534"

    workflow_diff imageset-config-headsonly.yaml "test-catalog-prune-diff" -c="--source-use-http"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 foo.v0.2.0" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
    check_image_removed "localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:535b8534"
}

# Test heads-only mode with catalogs that prune bundles
function pruned_catalogs_mirror_to_mirror() {
    workflow_mirror2mirror imageset-config-headsonly.yaml "test-catalog-prune" -c="--source-use-http"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 foo.v0.1.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
    check_image_exists "localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:535b8534"

    workflow_mirror2mirror imageset-config-headsonly.yaml "test-catalog-prune-diff" -c="--source-use-http"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 foo.v0.2.0" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
    check_image_removed "localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:535b8534"
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
    check_image_exists "localhost.localdomain:${REGISTRY_DISCONN_PORT}/stefanprodan/podinfo:6.0.0"
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
