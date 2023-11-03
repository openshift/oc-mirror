#!/bin/bash

set -eo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
cd "$SCRIPT_DIR/.." || exit 1

# This is used to download and install clients like the installer and oc
TESTED_OPENSHIFT_VERSION="${CI_OPENSHIFT_VERSION:-4.9}"
mirror_url="https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/stable-${TESTED_OPENSHIFT_VERSION}"

# These are used to map whether or not a package needs installed
declare -A dnf_provides
declare -a need_installed
dnf_provides=(
    [make]=/usr/bin/make
    [python39]=/usr/bin/python3.9
    [python39-devel]=/usr/include/python3.9/Python.h
    [unzip]=/usr/bin/unzip
    [git]=/usr/bin/git
    [rsync]=/usr/bin/rsync
    [gcc]=/usr/bin/gcc
    [libffi-devel]=/usr/include/ffi.h
    [openssl-devel]=/usr/include/openssl/ssl.h
    [skopeo]=/usr/bin/skopeo
)

# Map which packages need installed
for package in "${!dnf_provides[@]}"; do
    path="${dnf_provides[$package]}"
    if [ ! -f "$path" ]; then
        need_installed+=("$package")
    fi
done

# Install all missing packages
dnf -y install "${need_installed[@]}"

# Install OpenShift clients (oc and openshift-install)
pushd /usr/local/bin
for bin in install client; do
    curl -sLO "$mirror_url/openshift-$bin-linux.tar.gz"
    tar xvzf openshift-$bin-linux.tar.gz
    rm -f openshift-$bin-linux.tar.gz
done
chmod +x openshift-install oc kubectl
popd

# Cleanup
dnf clean all -y
