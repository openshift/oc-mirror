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

# check_bundles ensures the number and names of bundles in index_path
# matches that of exp_bundles_list.
function check_bundles() {
  local index_path="${1:?index path required}"
  local exp_bundles_list="${2:?expected bundles list must be set}"
  declare -A exp_bundles_set
  for bundle in $exp_bundles_list; do
    exp_bundles_set[$bundle]=bundle
  done
  local index_bundles=$(cat "$index_path" | jq -sr '.[] | select(.schema == "olm.bundle") | .name')

  # Ensure the number of bundles matches.
  local num_bundles=$(echo $index_bundles | wc -w)
  if (( ${#exp_bundles_set[@]} != $num_bundles )); then
    echo "number of bundles mirrored (${#exp_bundles_set[@]}) does not match expected number (${num_bundles})"
    return 1
  fi

  # Ensure each bundle is an expected bundle.
  for bundle in $index_bundles; do
    if [[ "${exp_bundles_set[$bundle]}" != "bundle" ]]; then
      echo "bundle $bundle not in expected bundle set"
      return 1
    fi
  done
}
