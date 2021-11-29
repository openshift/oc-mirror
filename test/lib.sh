#!/usr/bin/env bash

# run_cmd runs $CMD <args> <test flags> where test flags are arguments
# needed to run against a local test registry and provide informative
# debug data in case of test errors.
function run_cmd() {
  local test_flags="--log-level debug --source-skip-tls --dest-skip-tls --skip-cleanup"

  echo "$CMD" $@ $test_flags
  echo
  "$CMD" $@ $test_flags
}

# check_bundles ensures the number and names of bundles in catalog_image's index.json
# matches that of exp_bundles_list, and that all bundle images are pullable.
function check_bundles() {
  local catalog_image="${1:?catalog image required}"
  local exp_bundles_list="${2:?expected bundles list must be set}"
  local disconn_registry="${3:?disconnected registry host name must be set}"
  local ns="${4:-""}"

  crane export $catalog_image temp.tar
  local index_dir="${DATA_TMP}/unpacked"
  mkdir -p "$index_dir"
  local index_path="${index_dir}/index.json"
  tar xvf temp.tar /configs/index.json --strip-components=1 
  mv index.json $index_dir
  rm -f temp.tar

  declare -A exp_bundles_set
  for bundle in $exp_bundles_list; do
    exp_bundles_set[$bundle]=bundle
  done

# TODO: Use crane manifest to replace docker
 # local manifest=$(docker manifest inspect --insecure $catalog_image | jq .manifests | jq '.[].platform.architecture')
 # local num_manifest=$(echo $manifest | wc -w)
 # if (( $num_manifest != 4 )); then 
 #   echo "number of manifests in catalog $num_manifest does not match expected number 4"
 #   return 1
 # fi

  # Ensure the number of bundles matches.
  local index_bundle_names=$(cat "$index_path" | jq -sr '.[] | select(.schema == "olm.bundle") | .name')
  local num_bundles=$(echo $index_bundle_names | wc -w)
  if (( ${#exp_bundles_set[@]} != $num_bundles )); then
    echo "number of bundles mirrored (${#exp_bundles_set[@]}) does not match expected number (${num_bundles})"
    return 1
  fi

  # Ensure all bundle images are pullable.
  local index_bundle_images=$(cat "$index_path" | jq -sr '.[] | select(.schema == "olm.bundle") | .image')
   if [[ ! -z $ns ]]; then
    NS="$ns/"
  fi
  for image in $index_bundle_images; do
<<<<<<< HEAD
    image=${disconn_registry}/${NS}$(echo $image | cut --complement -d'/' -f1)
    if ! crane digest $image; then
=======
    image=${disconn_registry}/$(echo $image | cut -d'/' -f2-)
    if ! docker pull $image; then
>>>>>>> 3e52451 (Update to support e2e testing on mac)
      echo "bundle image $image not pushed to registry"
      return 1
    fi
  done

  # Ensure each bundle is an expected bundle.
  for bundle in $index_bundle_names; do
    if [[ "${exp_bundles_set[$bundle]}" != "bundle" ]]; then
      echo "bundle $bundle not in expected bundle set"
      return 1
    fi
  done
}

# cleanup will kill any running registry processes
function cleanup() {
    [[ -n $PID_DISCONN ]] && kill $PID_DISCONN
    [[ -n $PID_CONN ]] && kill $PID_CONN
}

# install_deps will install crane and registry2 in go bin dir
function install_deps() {
  pushd ${DATA_TMP}
  GOFLAGS=-mod=mod go install github.com/google/go-containerregistry/cmd/crane@latest
  popd
  crane export registry:2 registry2.tar
  tar xvf registry2.tar bin/registry
  mv bin/registry $GOBIN
  rm -f registry2.tar
}

# setup_reg will configure and start registry2 processes
# for connected and disconnected environments
function setup_reg() {
  # Setup connected registry
  echo -e "Setting up registries"
  cp ./test/e2e-config.yaml ${DATA_TMP}/conn.yaml
  find "${DATA_TMP}" -type f -exec sed -i -E 's@TMP@'"${REGISTRY_CONN_DIR}{"'@g' {} \;
  find "${DATA_TMP}" -type f -exec sed -i -E 's@PORT@'"${REGISTRY_CONN_PORT}"'@g' {} \;
  DPORT=$(expr ${REGISTRY_CONN_PORT} + 10)
  find "${DATA_TMP}" -type f -exec sed -i -E 's@DEBUG@'"$DPORT"'@g' {} \;
  registry serve ${DATA_TMP}/conn.yaml &> ${DATA_TMP}/coutput.log &
  PID_CONN=$!
  # Setup disconnected registry
  cp ./test/e2e-config.yaml ${DATA_TMP}/disconn.yaml
  find "${DATA_TMP}" -type f -exec sed -i -E 's@TMP@'"${REGISTRY_DISCONN_DIR}"'@g' {} \;
  find "${DATA_TMP}" -type f -exec sed -i -E 's@PORT@'"${REGISTRY_DISCONN_PORT}"'@g' {} \;
  DPORT=$(expr $REGISTRY_DISCONN_PORT + 10)
  find "${DATA_TMP}" -type f -exec sed -i -E 's@DEBUG@'"${DPORT}"'@g' {} \;
  registry serve ${DATA_TMP}/disconn.yaml &> ${DATA_TMP}/doutput.log &
  PID_DISCONN=$!

  echo -e "disconnected registry PID: $PID_DISCONN"
  echo -e "connected registry PID: $PID_CONN"
}

# prep_registry will copy the needed catalog image
# to the connected registry
function prep_registry() {
  local diff="${1:?diff required}"
  # Copy target catalog to connected registry
  if [[ $diff == "false" ]]; then
    crane copy quay.io/${CATALOGNAMESPACE}:test-catalog-latest \
    localhost:${REGISTRY_CONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest
  else
    crane copy quay.io/${CATALOGNAMESPACE}:test-catalog-diff \
    localhost:${REGISTRY_CONN_PORT}/${CATALOGNAMESPACE}:test-catalog-latest
  fi
}

# run_full will simulate an initial oc-mirror run
# to disk then publish to registry
function run_full() {
  local config="${1:?config required}"
  local diff="${2:?diff required}"
  local ns="${3:-""}"
  mkdir $PUBLISH_FULL_DIR
  # Copy the catalog to the connected registry so they can have the same tag
  "${DIR}/operator/setup-testdata.sh" "${DATA_TMP}" "$CREATE_FULL_DIR" "latest/$config" false
   prep_registry false
  run_cmd --config "${CREATE_FULL_DIR}/$config" "file://${CREATE_FULL_DIR}"
  pushd $PUBLISH_FULL_DIR
  if [[ ! -z $ns ]]; then
    NS="/$ns"
  fi
  run_cmd --from "${CREATE_FULL_DIR}/mirror_seq1_000000.tar" "docker://localhost:${REGISTRY_DISCONN_PORT}${NS}"
  popd
}

# run_diff will simulate a differential oc-mirror run
# to disk and then publish to registry
function run_diff() {
  local config="${1:?config required}"
  local ns="${2:-""}"
  mkdir $PUBLISH_DIFF_DIR
  # Copy the catalog to the connected registry so they can have the same tag
  "${DIR}/operator/setup-testdata.sh" "${DATA_TMP}" "$CREATE_DIFF_DIR" "latest/$config" true
  prep_registry true
  run_cmd --config "${CREATE_DIFF_DIR}/$config" "file://${CREATE_DIFF_DIR}"
  pushd ${PUBLISH_DIFF_DIR}
  if [[ ! -z $ns ]]; then
    NS="/$ns"
  fi
  run_cmd --from "${CREATE_DIFF_DIR}/mirror_seq2_000000.tar" "docker://localhost:${REGISTRY_DISCONN_PORT}${NS}"
  popd
}

# mirror2mirror will simulate oc-mirror
# mirror to mirror operations
function mirror2mirror() {
  local config="${1:?config required}"
  local ns="${2:-""}"
  # Copy the catalog to the connected registry so they can have the same tag
  "${DIR}/operator/setup-testdata.sh" "${DATA_TMP}" "${CREATE_FULL_DIR}" "latest/$config" false
  prep_registry false
  pushd ${CREATE_FULL_DIR}
  if [[ ! -z $ns ]]; then
    NS="/$ns"
  fi
  run_cmd --config "${CREATE_FULL_DIR}/$config" "docker://localhost:${REGISTRY_DISCONN_PORT}${NS}"
  popd
}