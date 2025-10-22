#!/usr/bin/env bash

# simple steps to build a single arch image and copy it to the ./layout directory

export DOCKER_DEFAULT_PLATFORM=linux/amd64

docker build -t testonly:v1.0.0 .

skopeo copy --src-tls-verify=false docker-daemon:testonly:v1.0.0 oci:layout --all --format v2s2
