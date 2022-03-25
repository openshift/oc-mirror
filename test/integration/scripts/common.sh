#!/bin/bash

# This common file is intended for scripts run inside the integration environment

cd "$(dirname "$SCRIPT_DIR")/.." || exit 1

if [ -z "${OPENSHIFT_CI}" ]; then
    echo "Not running in CI environment!" >&2
    exit 1
fi

# Source the virtual environment for all actions inside the integration area
source .venv/bin/activate

# The version of OpenShift to run with through the test suite with this version
#   of oc-mirror (noting that it can be different than the target release for this
#   version of oc-mirror)
TESTED_OPENSHIFT_VERSION="${CI_OPENSHIFT_VERSION:-4.9}"
export TESTED_OPENSHIFT_VERSION

# Secrets, as made available by CI
CONSOLE_REDHAT_COM_PULL_SECRET="$(cat /etc/ci/pull-secret/CONSOLE_REDHAT_COM_PULL_SECRET)"
export CONSOLE_REDHAT_COM_PULL_SECRET
AWS_ACCESS_KEY_ID="$(cat /etc/ci/aws-creds/AWS_ACCESS_KEY_ID)"
export AWS_ACCESS_KEY_ID
AWS_SECRET_ACCESS_KEY="$(cat /etc/ci/aws-creds/AWS_SECRET_ACCESS_KEY)"
export AWS_SECRET_ACCESS_KEY

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

declare -A artifact_files
artifact_files=(
    [oc-mirror.log]=output/.oc-mirror.log
    [openshift_install.log]=output/install/.openshift_install.log
)
export artifact_files

function maybe_cp () {
    echo -n " - checking for $1"
    if [ -f "$1" ]; then
        echo " (found -> $2)"
        cp "$1" "$2"
    else
        echo " (not found)"
        return 1
    fi
}

