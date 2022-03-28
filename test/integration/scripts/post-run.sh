#!/bin/bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
source "$SCRIPT_DIR/common.sh"

echo "Saving files to shared directory"
for file in "${!save_files[@]}"; do
    path="${save_files[$file]}"
    maybe_cp "$path" "${SHARED_DIR}/$file" ||:
done

echo "Saving run artifacts"
for file in "${!artifact_files[@]}"; do
    path="${artifact_files[$file]}"
    maybe_cp "$path" "${ARTIFACT_DIR}/$file" ||:
done
