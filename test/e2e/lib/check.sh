#!/usr/bin/env bash

# check_bundles ensures the number and names of bundles in catalog_image's index.json
# matches that of exp_bundles_list, and that all bundle images are pullable.
function check_bundles() {
  local catalog_image="${1:?catalog image required}"
  local exp_bundles_list="${2:?expected bundles list must be set}"
  local disconn_registry="${3:?disconnected registry host name must be set}"
  local ns="${4:-""}"

  crane export --insecure $catalog_image temp.tar
  local index_dir="${DATA_TMP}/unpacked"
  mkdir -p "$index_dir"
  local index_path="${index_dir}/index.json"
  tar xvf temp.tar /configs/index.json --strip-components=1 
  mv index.json $index_dir
  rm -f temp.tar

  opm validate $index_dir

  declare -A exp_bundles_set
  for bundle in $exp_bundles_list; do
    exp_bundles_set[$bundle]=bundle
  done

  # The test catalogs are not mult-architecture. The built images will contain a fat manifest with just the amd64 platform and linux OS
  # if the source image is not a fat manifest.
  local manifest=$(crane manifest --insecure --platform all $catalog_image | jq .manifests | jq '.[].platform.architecture')
  local num_manifest=$(echo $manifest | wc -w)
  if (( $num_manifest != 1 )); then 
    echo "number of manifests in catalog $num_manifest does not match expected number 1"
    return 1
  fi

  # Ensure the number of bundles matches.
  local index_bundle_names=$(cat "$index_path" | jq -sr '.[] | select(.schema == "olm.bundle") | .name')
  local num_bundles=$(echo $index_bundle_names | wc -w)
  if (( ${#exp_bundles_set[@]} != $num_bundles )); then
    echo "number of bundles mirrored (${num_bundles}) does not match expected number (${#exp_bundles_set[@]})"
    return 1
  fi

  # Ensure all bundle images are pullable.
  local index_bundle_images=$(cat "$index_path" | jq -sr '.[] | select(.schema == "olm.bundle") | .image')
   if [[ ! -z $ns ]]; then
    NS="$ns/"
  fi
  for image in $index_bundle_images; do
    image=${disconn_registry}/${NS}$(echo $image | cut --complement -d'/' -f1)
    if ! crane digest $image --insecure; then
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

# check_images_exists ensures the image(s) is found and pullable.
function check_image_exists() {
  local expected_image="${1:?expected image required}"
   if ! crane digest --insecure $expected_image; then
      echo "image $expected_image not pushed to registry"
      return 1
    fi
}

# check_sequence_number will inspect the number of pastMirrors / sequence number of the publish .metadata.json file against a user provided number
function check_sequence_number() {
  local expected_past_mirrors="${1:?expected past mirrors required}"
  local actual_past_mirrors=$(cat "${DATA_TMP}"/publish/.metadata.json | jq '.pastMirror.sequence')
  if [[ "$expected_past_mirrors" != "$actual_past_mirrors" ]]; then
    echo "expected_past_mirrors does not match actual_past_mirrors"
    return 1
  fi
}

# check_image_removed will check if an image has been pruned from the registry
function check_image_removed() {
   local removed_image="${1:?removed image required}"
   set -e
   output=$(crane digest --insecure $removed_image 2>&1) && returncode=$? || returncode=$?
   if [[ $returncode != 1 ]]; then
      echo "image $removed_image still exists in registry"
      return 1
    fi
}