#!/bin/bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
# The runtime used to build the image
runtime="${CONTAINER_RUNTIME:-podman}"


# Clean any artifacts of previous integration runs
cd "$SCRIPT_DIR/.."
make realclean
# Clean any build artifacts to ensure a clean build
cd ../..
make clean

$runtime build . -f images/tests/Dockerfile.integration -t oc-mirror-ci-integration
