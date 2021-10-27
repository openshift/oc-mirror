#!/usr/bin/env bash

: ${1:?registry name required}

REGISTRY_NAME=$1

#echo "Cleaning up docker registry $REGISTRY_NAME"

#docker stop $REGISTRY_NAME >/dev/null 2>&1 || true
#docker rm $REGISTRY_NAME >/dev/null 2>&1 || true
