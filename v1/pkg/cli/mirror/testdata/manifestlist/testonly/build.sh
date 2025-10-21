#!/usr/bin/env bash

# This script assumes the following:
# 1) you are running docker registry locally (e.g. docker run -d -p 5000:5000 --name registry registry:2)
# 2) you have docker buildx setup so that it can access the host network so you can push to localhost:5000
#    (e.g. docker buildx create --use desktop-linux --driver-opt network=host)

docker buildx build --platform linux/amd64,linux/ppc64le,linux/s390x -t localhost:5000/testonly:v1.0.0 --push .

skopeo copy --src-tls-verify=false docker://localhost:5000/testonly:v1.0.0 oci:layout --all --format v2s2