#!/bin/bash

set -e

valid_host_names=(registry proxy bastion)
validate () {
    for valid_host_name in "${valid_host_names[@]}"; do
        if [ "$1" = "$valid_host_name" ]; then
            return 0
        fi
    done
    return 1
}
node="${1?' You must provide a host to connect to'}" && validate "$1" || { echo 'Please supply a valid hostname as an argument.' >&2; exit 1; }

if [ -f output/cluster_name ]; then
    cluster_name=$(tr -d '[:space:]' < output/cluster_name)
else
    cluster_name=disco
fi

cluster_domain=$(grep '^cluster_domain:' vars.yml | cut -d: -f2- | tr -d ' ')

ssh_args=(
    -i "output/${cluster_name}_ed25519"
    -o IdentitiesOnly=yes
    -o StrictHostKeyChecking=no
    -o UserKnownHostsFile=/dev/null
    -q
)

if [ "$node" = "bastion" ]; then
    ssh_args+=(
        -o ProxyCommand="ssh -W %h:%p ${ssh_args[@]} ec2-user@proxy.$cluster_name.$cluster_domain"
    )
fi

export TERM=xterm-256color
exec ssh "${ssh_args[@]}" "ec2-user@$node.$cluster_name.$cluster_domain"
