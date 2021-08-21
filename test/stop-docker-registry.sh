#!/usr/bin/env bash

: ${1:?registry name required}

REGISTRY_NAME=$1

echo -e "\nCleaning up docker registry $REGISTRY_NAME"

docker stop $REGISTRY_NAME || true
docker rm $REGISTRY_NAME || true
