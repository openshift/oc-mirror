#!/bin/bash

# This script is designed to run outside fo the CI image. It builds the CI image and
#   stages a ci-like environment locally using your container runtime to validate that
#   the tests execute correctly. Mostly it is to make sure that the container images
#   are suitable, with minimal files share between, for multi-stage builds in OpenShift CI

set -eo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
# The runtime used to execute the image
runtime="${CONTAINER_RUNTIME:-podman}"

cd "$SCRIPT_DIR/.."

# Build a fresh copy of the integration image
"$SCRIPT_DIR/build-image.sh"

# Make directories necessary for mocking CI
mkdir -p ci/artifacts ci/shared ci/secrets/pull-secret ci/secrets/aws-creds

# Secret setup
if [ -z "$CONSOLE_REDHAT_COM_PULL_SECRET" ]; then
    read -rsp "Pull secret: " CONSOLE_REDHAT_COM_PULL_SECRET
fi
echo "$CONSOLE_REDHAT_COM_PULL_SECRET" > ci/secrets/pull-secret/CONSOLE_REDHAT_COM_PULL_SECRET
if [ -z "$AWS_ACCESS_KEY_ID" ]; then
    read -rsp "AWS Access key ID: " AWS_ACCESS_KEY_ID
fi
echo "$AWS_ACCESS_KEY_ID" > ci/secrets/aws-creds/AWS_ACCESS_KEY_ID
if [ -z "$AWS_SECRET_ACCESS_KEY" ]; then
    read -rsp "AWS Secret access key: " AWS_SECRET_ACCESS_KEY
fi
echo "$AWS_SECRET_ACCESS_KEY" > ci/secrets/aws-creds/AWS_SECRET_ACCESS_KEY

# Execute commands inside the integration test folder in the container, with
#  bind mounts for mocked artifacts, shared directories, and secrets
function ci_run() {
    $runtime run --rm -it -e OPENSHIFT_CI=true -e SHARED_DIR="/shared" -e ARTIFACT_DIR="/artifacts" -v ./ci/artifacts:/artifacts -v ./ci/shared:/shared -v ./ci/secrets:/etc/ci --security-opt=label=disable --privileged oc-mirror-ci-integration /bin/sh -c "chown -R 1000:1000 . /shared /artifacts && cd test/integration && $* && cd ../.. && chown -R 0:0 . /shared /artifacts"
}

# Remove the infra no matter what happens in the tests
function ci_cleanup() {
    ci_run make ci-delete
}
trap ci_cleanup EXIT

# Run the ci test target
ci_run make ci
