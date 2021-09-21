#!/usr/bin/env bash

# run_cmd runs $CMD <args> <test flags> where test flags are arguments
# needed to run against a local test registry and provide informative
# debug data in case of test errors.
function run_cmd() {
  local test_flags="--log-level debug --skip-tls --skip-cleanup"

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

  docker pull $catalog_image
  local container=$(docker create $catalog_image)
  local index_dir="${DATA_TMP}/unpacked"
  mkdir -p "$index_dir"
  local index_path="${index_dir}/index.json"
  docker cp ${container}:/configs/index.json "$index_path"

  declare -A exp_bundles_set
  for bundle in $exp_bundles_list; do
    exp_bundles_set[$bundle]=bundle
  done

  # Ensure the number of bundles matches.
  local index_bundle_names=$(cat "$index_path" | jq -sr '.[] | select(.schema == "olm.bundle") | .name')
  local num_bundles=$(echo $index_bundle_names | wc -w)
  if (( ${#exp_bundles_set[@]} != $num_bundles )); then
    echo "number of bundles mirrored (${#exp_bundles_set[@]}) does not match expected number (${num_bundles})"
    return 1
  fi

  # Ensure all bundle images are pullable.
  local index_bundle_images=$(cat "$index_path" | jq -sr '.[] | select(.schema == "olm.bundle") | .image')
  for image in $index_bundle_images; do
    image=${disconn_registry}/$(echo $image | cut --complement -d'/' -f1)
    if ! docker pull $image; then
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
