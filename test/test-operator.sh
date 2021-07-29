#!/usr/bin/env bash

set -eu

CMD="${1:?cmd bin path is required}"

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
DATA_TMP=$(mktemp -d "${DIR}/operator-test.XXXXX")
OUTPUT_DIR="${DATA_TMP}/output"

trap "rm -rf $DATA_TMP; ${DIR}/stop-docker-registry.sh" EXIT

"${DIR}/start-docker-registry.sh"
"${DIR}/operator/setup-testdata.sh" "$DATA_TMP" "$OUTPUT_DIR"
"$CMD" create full --dir "$OUTPUT_DIR" --log-level debug --skip-tls
