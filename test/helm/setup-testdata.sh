#!/usr/bin/env bash

set -eu

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DATA_DIR="${1:?data dir is required}"
OUTPUT_DIR="${2:?output dir is required}"
CONFIG_PATH="${3:?config path is required}"
CHART_PATH="${4:?chart path is required}"

function setup() {
  echo -e "\nSetting up test directory in $DATA_DIR"
  mkdir -p "$OUTPUT_DIR"
  cp "${DIR}/testdata/configs/${CONFIG_PATH}" "${OUTPUT_DIR}/"
  cp "${DIR}/testdata/charts/${CHART_PATH}" "${DATA_DIR}/"
  find "$DATA_DIR" -type f -exec sed -i -E 's@DATA_TMP@'"$DATA_DIR"'@g' {} \;
}

setup