#!/usr/bin/env bash

# workflow_full will simulate an initial oc-mirror run
# to disk then publish to registry
function workflow_full() {
  parse_args "$@"
  local config="${1:?config required}"
  mkdir $PUBLISH_FULL_DIR
  # Copy the catalog to the connected registry so they can have the same tag
  setup_operator_testdata "${DATA_TMP}" "$CREATE_FULL_DIR" "$config" false
  prep_registry false
  run_cmd --config "${CREATE_FULL_DIR}/$config" "file://${CREATE_FULL_DIR}" $CREATE_FLAGS
  pushd $PUBLISH_FULL_DIR
  if $DIFF; then
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
  mkdir $PUBLISH_DIFF_DIR
  # Copy the catalog to the connected registry so they can have the same tag
  setup_operator_testdata "${DATA_TMP}" "$CREATE_DIFF_DIR" "$config" true
  prep_registry true
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
  mkdir $PUBLISH_FULL_DIR
  # Copy the catalog to the connected registry so they can have the same tag
  setup_operator_testdata "${DATA_TMP}" "$CREATE_FULL_DIR" "$config" false
  prep_registry false
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
   # Copy the catalog to the connected registry so they can have the same tag
  setup_operator_testdata "${DATA_TMP}" "${CREATE_FULL_DIR}" "$config" false
  prep_registry false
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
  prep_registry false
  run_cmd --config "${CREATE_FULL_DIR}/$config" "file://${CREATE_FULL_DIR}" $CREATE_FLAGS
  pushd $PUBLISH_FULL_DIR
  run_cmd --from "${CREATE_FULL_DIR}/mirror_seq1_000000.tar" "docker://localhost.localdomain:${REGISTRY_DISCONN_PORT}${NS}" $PUBLISH_FLAGS
  popd
}

