#!/usr/bin/env bash

# workflow_full will simulate an initial oc-mirror run
# to disk then publish to registry
function workflow_full() {
  parse_args "$@"
  local config="${1:?config required}"
  local catalog_tag="${2:?catalog_tag required}"
  mkdir $PUBLISH_FULL_DIR
  prep_registry "${catalog_tag}"
  # Copy the catalog to the connected registry so they can have the same tag
  # Full is the only place catalog digests could be used so setting and using the
  # CATALOGDIGEST set by prep_registry here.
  setup_operator_testdata "${DATA_TMP}" "$CREATE_FULL_DIR" "$config" false $CATALOGDIGEST
  run_cmd --config "${CREATE_FULL_DIR}/$config" "file://${CREATE_FULL_DIR}" $CREATE_FLAGS
  pushd $PUBLISH_FULL_DIR
  if !$DIFF; then
    cleanup_conn
  fi
  run_cmd --from "${CREATE_FULL_DIR}/mirror_seq1_000000.tar" "docker://localhost.localdomain:${REGISTRY_DISCONN_PORT}${NS}" $PUBLISH_FLAGS
  popd
}

# workflow_diff will simulate a differential oc-mirror run
# to disk and then publish to registry
function workflow_diff() {
  parse_args "$@"
  local config="${1:?config required}"
  local catalog_tag="${2:?catalog_tag required}"
  mkdir $PUBLISH_DIFF_DIR
  # Copy the catalog to the connected registry so they can have the same tag
  setup_operator_testdata "${DATA_TMP}" "$CREATE_DIFF_DIR" "$config" true
  prep_registry "${catalog_tag}"
  run_cmd --config "${CREATE_DIFF_DIR}/$config" "file://${CREATE_DIFF_DIR}" $CREATE_FLAGS
  pushd ${PUBLISH_DIFF_DIR}
  run_cmd --from "${CREATE_DIFF_DIR}/mirror_seq2_000000.tar" "docker://localhost.localdomain:${REGISTRY_DISCONN_PORT}${NS}" $PUBLISH_FLAGS
  popd
}

# workflow_no_updates will simulate an initial oc-mirror run
# to disk then resubmit another oc-mirror which should yield no changes
function workflow_no_updates() {
  parse_args "$@"
  local config="${1:?config required}"
  local catalog_tag="${2:?catalog_tag required}"
  mkdir $PUBLISH_FULL_DIR
  # Copy the catalog to the connected registry so they can have the same tag
  setup_operator_testdata "${DATA_TMP}" "$CREATE_FULL_DIR" "$config" false
  prep_registry "${catalog_tag}"
  run_cmd --config "${CREATE_FULL_DIR}/$config" "file://${CREATE_FULL_DIR}" $CREATE_FLAGS
  run_cmd --config "${CREATE_FULL_DIR}/$config" "file://${CREATE_FULL_DIR}" $CREATE_FLAGS
  pushd $PUBLISH_FULL_DIR
  run_cmd --from "${CREATE_FULL_DIR}/mirror_seq1_000000.tar" "docker://localhost.localdomain:${REGISTRY_DISCONN_PORT}${NS}" $PUBLISH_FLAGS
  popd
}

# workflow_mirror2mirror will simulate oc-mirror
# mirror to mirror operations
function workflow_mirror2mirror() {
  parse_args "$@"
  local config="${1:?config required}"
  local catalog_tag="${2:?catalog_tag required}"
   # Copy the catalog to the connected registry so they can have the same tag
  setup_operator_testdata "${DATA_TMP}" "${CREATE_FULL_DIR}" "$config" false
  prep_registry "$catalog_tag"
  pushd ${CREATE_FULL_DIR}
  run_cmd --config "${CREATE_FULL_DIR}/$config" "docker://localhost.localdomain:${REGISTRY_DISCONN_PORT}${NS}" $CREATE_FLAGS
  popd
}

# workflow_helm will run a helm mirror with helm setup
# TODO: add a way to do dynamic environment setup to
# remove this extra function
function workflow_helm() {
  parse_args "$@"
  local config="${1:?config required}"
  local chart="${2:?chart required}"
  mkdir $PUBLISH_FULL_DIR
  # Copy the helm chart and config in workspace
  setup_helm_testdata "${DATA_TMP}" "$CREATE_FULL_DIR" "$config" "$chart"
  run_cmd --config "${CREATE_FULL_DIR}/$config" "file://${CREATE_FULL_DIR}" $CREATE_FLAGS
  pushd $PUBLISH_FULL_DIR
  run_cmd --from "${CREATE_FULL_DIR}/mirror_seq1_000000.tar" "docker://localhost.localdomain:${REGISTRY_DISCONN_PORT}${NS}" $PUBLISH_FLAGS
  popd
}

function workflow_oci_copy() {
  parse_args "$@" 
  local config="${1:?config required}"
  local catalog_tag="${2:?catalog_tag required}"
  local oci_fbc="${3:?oci_fbc required}"
  # Prepare test data
  setup_operator_testdata "${DATA_TMP}" "${MIRROR_OCI_DIR}" "${config}" false 
  
  # setup_reg already called in e2e-simple.sh
  # Copy the test catalog to a local connected registry
  prep_registry "${catalog_tag}" #catalog image quay.io/redhatgov/oc-mirror-dev:<tag>
  # call oc-mirror
  run_cmd --config "${MIRROR_OCI_DIR}/${config}" $CREATE_FLAGS "${oci_fbc}"
}

function workflow_m2m_oci_catalog() {
  parse_args "$@" 
  local config="${1:?config required}"
  local remote_image="${2:?remote_image required}"
  prepare_oci_testdata "${DATA_TMP}"
  prepare_mirror_testdata "${DATA_TMP}" "${MIRROR_OCI_DIR}" "${config}" false 
 
  # call oc-mirror
  run_cmd --config "${MIRROR_OCI_DIR}/${config}" $CREATE_FLAGS "${remote_image}"
}

function workflow_m2d2m_oci_catalog() {

  parse_args "$@"
  local config="${1:?config required}"
  local remote_image="${2:?remote_image required}"

  mkdir $PUBLISH_FULL_DIR
  prepare_oci_testdata "${DATA_TMP}"
  prepare_mirror_testdata "${DATA_TMP}" "${MIRROR_OCI_DIR}" "${config}" false 
  
  run_cmd --config "${MIRROR_OCI_DIR}/${config}" "file://${CREATE_FULL_DIR}" $CREATE_FLAGS
  pushd $PUBLISH_FULL_DIR
  if !$DIFF; then
    cleanup_conn
  fi
  run_cmd --from "${CREATE_FULL_DIR}/mirror_seq1_000000.tar" "docker://${remote_image}"

  popd
}

function workflow_oci_mirror_all() {
  parse_args "$@" 
  local config="${1:?config required}"
  local remote_image="${2:?remote_image required}"
  prepare_oci_testdata "${DATA_TMP}"
  prepare_mirror_testdata "${DATA_TMP}" "${MIRROR_OCI_DIR}" "${config}" false 
  echo $config $remote_image 

  # call oc-mirror
  run_cmd --config "${MIRROR_OCI_DIR}/${config}" $CREATE_FLAGS "${remote_image}"
}
