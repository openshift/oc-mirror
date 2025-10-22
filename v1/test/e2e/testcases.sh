#!/usr/bin/env bash

# These are used to define all testcase
# run during end to end test
declare -a TESTCASES
TESTCASES[1]="full_catalog"
TESTCASES[2]="full_catalog_with_digest"
TESTCASES[3]="headsonly_diff"
TESTCASES[4]="pruned_catalogs"
TESTCASES[5]="pruned_catalogs_mirror_to_mirror"
TESTCASES[6]="pruned_catalogs_with_target"
TESTCASES[7]="pruned_catalogs_with_include"
TESTCASES[8]="registry_backend"
TESTCASES[9]="mirror_to_mirror"
TESTCASES[10]="mirror_to_mirror_nostorage"
TESTCASES[11]="custom_namespace"
TESTCASES[12]="package_filtering"
TESTCASES[13]="single_version"
TESTCASES[14]="version_range"
TESTCASES[15]="max_version"
TESTCASES[16]="skip_deps"
TESTCASES[17]="helm_local"
TESTCASES[18]="no_updates_exist"
TESTCASES[19]="m2m_oci_catalog"
TESTCASES[20]="m2m_release_with_oci_catalog"
TESTCASES[21]="headsonly_diff_with_target"
TESTCASES[22]="helm_repository"
TESTCASES[23]="m2d2m_oci_catalog"


# Test full catalog mode.
function full_catalog() {
    workflow_full imageset-config-full.yaml "test-catalog-latest" -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.0.0 baz.v1.0.1 baz.v1.1.0 foo.v0.1.0 foo.v0.2.0 foo.v0.3.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test full catalog mode with digest.
function full_catalog_with_digest() {
    workflow_full imageset-config-full-digest.yaml "test-catalog-latest" -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    TMPTAG=$(echo $CATALOGDIGEST | cut -d: -f 2)
    ## We default to 6 for the partial digest length to get unique tags per repo
    TMPTAG=${TMPTAG:0:6}
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:${TMPTAG}\
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.0.0 baz.v1.0.1 baz.v1.1.0 foo.v0.1.0 foo.v0.2.0 foo.v0.3.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test heads-only mode
function headsonly_diff () {
    workflow_full imageset-config-headsonly.yaml "test-catalog-latest" --diff -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}

    workflow_diff imageset-config-headsonly.yaml "test-catalog-diff" -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.0 foo.v0.3.1 foo.v0.3.2" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test heads-only mode with target
function headsonly_diff_with_target () {
    workflow_full imageset-config-headsonly-newtarget.yaml "test-catalog-latest" --diff -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    # shellcheck disable=SC2086
    check_bundles localhost.localdomain:"${REGISTRY_DISCONN_PORT}"/"${CATALOGORG}"/${TARGET_CATALOG_NAME}:${TARGET_CATALOG_TAG} \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}

    workflow_diff imageset-config-headsonly-newtarget.yaml "test-catalog-diff" -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:"${REGISTRY_DISCONN_PORT}"/"${CATALOGORG}"/"${TARGET_CATALOG_NAME}":"${TARGET_CATALOG_TAG}" \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.0 foo.v0.3.1 foo.v0.3.2" \
    localhost.localdomain:"${REGISTRY_DISCONN_PORT}"
}

# Test heads-only mode with catalogs that prune bundles
function pruned_catalogs() {
    workflow_full imageset-config-headsonly.yaml "test-catalog-prune" --diff -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 foo.v0.1.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
    check_image_exists "localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:${CATALOG_ID}"

    workflow_diff imageset-config-headsonly.yaml "test-catalog-prune-diff" -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 foo.v0.2.0" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
    check_image_removed "localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:${CATALOG_ID}"
}

# Test heads-only mode with catalogs that prune with a custom target
# name set
function pruned_catalogs_with_target() {
    workflow_full imageset-config-headsonly-newtarget.yaml "test-catalog-prune" --diff -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGORG}/${TARGET_CATALOG_NAME}:${TARGET_CATALOG_TAG} \
    "bar.v0.1.0 foo.v0.1.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
    check_image_exists "localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:${CATALOG_ID}"

    workflow_diff imageset-config-headsonly-newtarget.yaml "test-catalog-prune-diff" -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGORG}/${TARGET_CATALOG_NAME}:${TARGET_CATALOG_TAG} \
    "bar.v0.1.0 foo.v0.2.0" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
    check_image_removed "localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:${CATALOG_ID}"
}

# Test heads-only mode with catalogs that prune bundles
function pruned_catalogs_with_include() {
    workflow_full imageset-config-filter-multi-prune.yaml "test-catalog-prune" --diff -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 foo.v0.1.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
    check_image_exists "localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:${CATALOG_ID}"

    workflow_diff imageset-config-filter-multi-prune.yaml "test-catalog-prune-diff" -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 foo.v0.2.0" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
    check_image_removed "localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:${CATALOG_ID}"
}

# Test heads-only mode with catalogs that prune bundles
function pruned_catalogs_mirror_to_mirror() {
    workflow_mirror2mirror imageset-config-headsonly.yaml "test-catalog-prune" -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 foo.v0.1.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
    check_image_exists "localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:${CATALOG_ID}"

    workflow_mirror2mirror imageset-config-headsonly.yaml "test-catalog-prune-diff" -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 foo.v0.2.0" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
    check_image_removed "localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:${CATALOG_ID}"
}

# Test registry backend
function registry_backend () {
    workflow_full imageset-config-headsonly-backend-registry.yaml "test-catalog-latest" --diff -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}

    workflow_diff imageset-config-headsonly-backend-registry.yaml "test-catalog-diff" -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.0 foo.v0.3.1 foo.v0.3.2" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test mirror to mirror with local backend
function mirror_to_mirror() {
    workflow_mirror2mirror imageset-config-headsonly.yaml "test-catalog-latest" -c="--source-use-http --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test mirror to mirror no backend
function mirror_to_mirror_nostorage() {
    workflow_mirror2mirror imageset-config-full.yaml "test-catalog-latest" -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.0.0 baz.v1.0.1 baz.v1.1.0 foo.v0.1.0 foo.v0.2.0 foo.v0.3.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test registry backend with custom namespace
function custom_namespace {
    workflow_full imageset-config-headsonly-backend-registry.yaml "test-catalog-latest" --diff -n="custom" -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles "localhost.localdomain:${REGISTRY_DISCONN_PORT}/custom/${CATALOGNAMESPACE}:test-catalog-latest" \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT} "custom"

    workflow_diff imageset-config-headsonly-backend-registry.yaml "test-catalog-diff" -n="custom" -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles "localhost.localdomain:${REGISTRY_DISCONN_PORT}/custom/${CATALOGNAMESPACE}:test-catalog-latest" \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.0 foo.v0.3.1 foo.v0.3.2" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT} "custom"
}


# Test package filtering
function package_filtering {
    workflow_full imageset-config-filter.yaml "test-catalog-latest" --diff -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles "localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest" \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 foo.v0.1.0 foo.v0.2.0 foo.v0.3.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}

    workflow_diff imageset-config-filter-multi.yaml "test-catalog-diff" -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles "localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest" \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.0.1 baz.v1.1.0 foo.v0.1.0 foo.v0.2.0 foo.v0.3.0 foo.v0.3.1 foo.v0.3.2" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test catalog with one version in a package specified
function single_version () {
    workflow_full imageset-config-filter-single.yaml "test-catalog-latest" -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "baz.v1.0.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test catalog with a version range in a package specified
function version_range () {
    workflow_full imageset-config-filter-range.yaml "test-catalog-latest" -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "foo.v0.2.0 foo.v0.3.0" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test catalog with a max version in a package specified
function max_version () {
    workflow_full imageset-config-filter-max.yaml "test-catalog-latest" -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "foo.v0.1.0 foo.v0.2.0 foo.v0.3.0" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}


# Test skip deps
function skip_deps {
    workflow_full imageset-config-skip-deps.yaml "test-catalog-latest" --diff -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles "localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest" \
    "bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}


# Test local helm chart
function helm_local {
    workflow_helm imageset-config-helm.yaml podinfo-6.0.0.tgz
    check_image_exists "localhost.localdomain:${REGISTRY_DISCONN_PORT}/stefanprodan/podinfo:6.0.0" "amd64"
}


# Test helm chart with URL
function helm_repository {
    workflow_helm_repository imageset-config-helm-repository.yaml
    check_image_exists "localhost.localdomain:${REGISTRY_DISCONN_PORT}/redhat-developer/servicebinding-operator@sha256:ac47f496fb7ecdcbc371f8c809fad2687ec0c35bbc8c522a7ab63b3e5ffd90ea"
}

# Test no updates
function no_updates_exist {
    workflow_no_updates imageset-config-headsonly.yaml "test-catalog-latest" --diff -c="--source-use-http  --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest \
    "bar.v0.1.0 bar.v0.2.0 bar.v1.0.0 baz.v1.1.0 foo.v0.3.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}

    if [ -f ${CREATE_FULL_DIR}/mirror_seq2_000000.tar ]; then
        echo "no updates should not have a second sequence"
        exit 1
    fi
    check_sequence_number 1
}

# Test OCI local catalog
function m2m_oci_catalog {
    rm -fr olm_artifacts
    workflow_m2m_oci_catalog imageset-config-oci-mirror.yaml "docker://localhost.localdomain:${REGISTRY_DISCONN_PORT}" -c="--dest-skip-tls --oci-insecure-signature-policy --rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${OCI_REGISTRY_NAMESPACE}/oc-mirror-dev:test-catalog-latest \
    "baz.v1.0.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}

# Test OCI local release,catalog,additionalImages
function m2m_release_with_oci_catalog {
    # setup url to lookup release info (certificate issued for localhost.localdomain)release-images:alpine-x86_64
    export UPDATE_URL_OVERRIDE="https://localhost.localdomain:3443/graph"
    # ensure cincinnati client does not reject the rquest - due to untrusted CA Authority
    export SSL_CERT_FILE=test/e2e/graph/server.crt
    # build and start the service

    go build -mod=readonly -o test/e2e/graph test/e2e/graph/main.go

    test/e2e/graph/main & PID_GO=$!
    echo -e "go cincinnatti web service PID: ${PID_GO}"
    # copy relevant files and start the mirror process
    workflow_oci_mirror_all imageset-config-oci-mirror-all.yaml "docker://localhost.localdomain:${REGISTRY_DISCONN_PORT}/test-catalog-latest" -c="--dest-skip-tls --oci-insecure-signature-policy --rebuild-catalogs --build-catalog-cache"

    # use crane digest to verify
    crane digest --insecure localhost.localdomain:${REGISTRY_DISCONN_PORT}/test-catalog-latest/${OCI_REGISTRY_NAMESPACE}/oc-mirror-dev:bar-v0.1.0
    crane digest --insecure localhost.localdomain:${REGISTRY_DISCONN_PORT}/test-catalog-latest/openshift/release-images:alpine-x86_64
    crane digest --insecure localhost.localdomain:${REGISTRY_DISCONN_PORT}/test-catalog-latest/openshift/release:alpine-x86_64-alpime

    rm -rf test/e2e/graph/main
    rm -rf test/e2e/graph/server*.*
    unset SSL_CERT_FILE
    unset UPDATE_URL_OVERRIDE
}


# Test full catalog mode.
function m2d2m_oci_catalog() {
    rm -fr olm_artifacts
    workflow_m2d2m_oci_catalog imageset-config-oci-mirror.yaml "localhost.localdomain:${REGISTRY_DISCONN_PORT}" -c="--source-use-http   --source-skip-tls --rebuild-catalogs --build-catalog-cache" -p="--rebuild-catalogs --build-catalog-cache"
    check_bundles localhost.localdomain:${REGISTRY_DISCONN_PORT}/${OCI_REGISTRY_NAMESPACE}/oc-mirror-dev:test-catalog-latest \
    "baz.v1.0.1" \
    localhost.localdomain:${REGISTRY_DISCONN_PORT}
}