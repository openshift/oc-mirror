#!/usr/bin/env bash

: ${1:?registry name required}
: ${2:?port required}

REGISTRY_NAME=$1
PORT=$2

echo "Starting containerized docker registry $REGISTRY_NAME listening at localhost:${PORT}"

docker run -d -p $PORT:5000 --restart always --name $REGISTRY_NAME registry:2
