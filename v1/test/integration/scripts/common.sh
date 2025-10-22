#!/bin/bash

# This common file is intended for scripts run inside the integration environment
if ! cd "$(dirname "$SCRIPT_DIR")"; then
    echo "Unable to change into the integration test directory" >&2
    exit 1
fi

if [[ -z "${OPENSHIFT_CI}" ]]; then
    echo "Not running in CI environment! Things may break!" >&2
fi

# Home is not being set for random UID in CI
HOME="${HOME:-/alabama}"
export HOME

# The version of OpenShift to run with through the test suite with this version
#   of oc-mirror (noting that it can be different than the target release for this
#   version of oc-mirror)
TESTED_OPENSHIFT_VERSION="${CI_OPENSHIFT_VERSION:-4.9}"
export TESTED_OPENSHIFT_VERSION

if [[ -n "$OPENSHIFT_CI" ]]; then
    # Secrets, as made available by CI
    CONSOLE_REDHAT_COM_PULL_SECRET="$(cat /etc/ci/pull-secret/CONSOLE_REDHAT_COM_PULL_SECRET)"
    export CONSOLE_REDHAT_COM_PULL_SECRET
    AWS_ACCESS_KEY_ID="$(cat /etc/ci/aws-creds/AWS_ACCESS_KEY_ID)"
    export AWS_ACCESS_KEY_ID
    AWS_SECRET_ACCESS_KEY="$(cat /etc/ci/aws-creds/AWS_SECRET_ACCESS_KEY)"
    export AWS_SECRET_ACCESS_KEY
else
    mkdir -p output/{shared,artifacts}
    export SHARED_DIR="$PWD/output/shared"
    export ARTIFACT_DIR="$PWD/output/artifacts"
fi

# The list of files needed to be saved and their locations
declare -A save_files
save_files=(
    [aws_credentials]=output/aws_credentials
    [cluster_name]=output/cluster_name
    [metadata.json]=output/install/metadata.json
    [terraform.cluster.tfstate]=output/install/terraform.cluster.tfstate
    [terraform.tfstate]=output/terraform/terraform.tfstate
)
export save_files

declare -a artifact_files
artifact_files=(
    output/oc-mirror-*.log
    output/install/.openshift_install.log
)
export artifact_files

function sanitize_kubeadmin () {
    sed 's/Login to the console with user: .*$/Login to the console with user: <REDACTED>/'
}

function maybe_cp () {
    src="$1"
    dest="$(dirname "$2")/$(basename "$2" | sed 's/^\.//')"
    echo -n " - checking for $src"
    if [[ -f "$src" ]]; then
        echo " (found -> $dest)"
        mkdir -p "$(dirname "$dest")"
        sanitize_kubeadmin < "$src" > "$dest"
    else
        echo " (not found)"
        return 1
    fi
}
