#!/usr/bin/env bash

set -eu

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DATA_DIR="${1:?data dir is required}"
OUTPUT_DIR="${2:?output dir is required}"
REGISTRY="localhost:5000"
CATALOGNAMESPACE="test-catalogs"
REGISTRY_CATALOGNAMESPACE="${REGISTRY}/${CATALOGNAMESPACE}"

function setup() {
  echo -e "\nSetting up test directory in $DATA_DIR"
  cp -r "${DIR}/testdata/bundles/"* "$DATA_DIR"
  mkdir -p "${DATA_DIR}/index"
  cp -r "${DIR}/testdata/indices/latest/"* "${DATA_DIR}/index/"
  find "$DATA_DIR" -type f -exec sed -i -E 's@REGISTRY_ONLY@'"$REGISTRY"'@g' {} \;
  mkdir -p "${OUTPUT_DIR}"
  cp "${DIR}/testdata/configs/latest/imageset-config.yaml" "$OUTPUT_DIR/"
  find "$DATA_DIR" -type f -exec sed -i -E 's@REGISTRY_CATALOGNAMESPACE@'"$REGISTRY_CATALOGNAMESPACE"'@g' {} \;
}

function build_push_bundles() {
  echo -e "\nBuilding and pushing bundle images"
  for d in `find "${DATA_DIR}" -maxdepth 1 -name *-bundle-*`; do
    local img="${REGISTRY}/$(basename $d | cut -d- -f1)-operator/$(basename $d | cut -d- -f1-2):$(basename $d | cut -d- -f3)"
    pushd $d
    docker build -t $img -f bundle.Dockerfile .
    docker push $img
    popd
  done
}

function build_push_related_images() {
  echo -e "\nBuilding and pushing related images"
  for img in `yq eval '.relatedImages[].image' "${DATA_DIR}/index/index.yaml" --no-doc`; do
    local tmp=$(mktemp -d ${DATA_DIR}/bundle-image.XXXXX)
    pushd "$tmp"
    echo -e "#!/bin/sh\n\necho \"relatedImage: $img\"" > run.sh
    chmod +x run.sh
    cat <<EOF > Dockerfile
FROM alpine
COPY run.sh /
ENTRYPOINT ["/run.sh"]
EOF
    docker build -t $img -f Dockerfile .
    docker push $img
    popd
    rm -rf "$tmp"
  done
}

# TODO(estroz): consider regenerating index.yaml with opm.
function build_push_catalog() {
  echo -e "\nBuilding and pushing catalog image"
  local img="${REGISTRY_CATALOGNAMESPACE}/test-catalog:latest"
  pushd "${DATA_DIR}/index"
  docker build -t $img -f index.Dockerfile .
  docker push $img
  popd
}

setup
build_push_bundles
build_push_related_images
build_push_catalog
