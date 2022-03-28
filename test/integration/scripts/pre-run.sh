#!/bin/bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
source "$SCRIPT_DIR/common.sh"

echo "Restoring files from shared directory"
for file in "${!save_files[@]}"; do
    path="${save_files[$file]}"
    maybe_cp "${SHARED_DIR}/$file" "$path" ||:
done
