#!/usr/bin/env bash
set -euo pipefail

command -v cosign >/dev/null || {
	echo "cosign is required" >&2
	exit 1
}

KEYS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../testdata/keys" && pwd)"

if [[ -f "${KEYS_DIR}/cosign.key" && -f "${KEYS_DIR}/cosign.pub" ]]; then
	echo "Cosign keys already exist in ${KEYS_DIR}/"
	echo "Delete them first if you want to regenerate."
	exit 0
fi

COSIGN_PASSWORD="" cosign generate-key-pair --output-key-prefix="${KEYS_DIR}/cosign"

echo "Wrote ${KEYS_DIR}/cosign.key"
echo "Wrote ${KEYS_DIR}/cosign.pub"
