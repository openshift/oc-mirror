#!/usr/bin/env bash

# These are used to define all testcase
# run during end to end test
declare -a TESTCASES
TESTCASES[1]="full_catalog"
TESTCASES[2]="full_catalog"
TESTCASES[3]="headsonly_diff"
TESTCASES[4]="pruned_catalogs"
TESTCASES[5]="pruned_catalogs_mirror_to_mirror"
TESTCASES[6]="pruned_catalogs_with_target"
TESTCASES[7]="registry_backend"
TESTCASES[8]="mirror_to_mirror"
TESTCASES[9]="mirror_to_mirror_nostorage"
TESTCASES[10]="custom_namespace"
TESTCASES[11]="package_filtering"
TESTCASES[12]="single_version"
TESTCASES[13]="version_range"
TESTCASES[14]="max_version"
TESTCASES[15]="skip_deps"
TESTCASES[16]="helm_local"
TESTCASES[17]="no_updates_exist"

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
    ## We default to 6 for the partial digest length to get unique tags per repo
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

# Test heads-only mode with catalogs that prune with a custom target
# name set
function pruned_catalogs_with_target() {
    workflow_full imageset-config-headsonly-newtarget.yaml "test-catalog-prune" --diff -c="--source-use-http"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGORG}/${TARGET_CATALOG_NAME}:${TARGET_CATALOG_TAG} \
    "bar.v0.1.0 foo.v0.1.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
    check_image_exists "localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:535b8534"

    workflow_diff imageset-config-headsonly-newtarget.yaml "test-catalog-prune-diff" -c="--source-use-http"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGORG}/${TARGET_CATALOG_NAME}:${TARGET_CATALOG_TAG} \
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

# Test catalog with one version in a package specified
function single_version () {
    workflow_full imageset-config-filter-single.yaml "test-catalog-latest" -c="--source-use-http"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "baz.v1.0.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test catalog with a version range in a package specified
function version_range () {
    workflow_full imageset-config-filter-range.yaml "test-catalog-latest" -c="--source-use-http"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "foo.v0.2.0 foo.v0.3.0" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test catalog with a max version in a package specified
function max_version () {
    workflow_full imageset-config-filter-max.yaml "test-catalog-latest" -c="--source-use-http"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "foo.v0.1.0 foo.v0.2.0 foo.v0.3.0" \
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

# Test no updates
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
