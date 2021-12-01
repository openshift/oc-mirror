#!/bin/sh
IMG=quay.io/samwalke/rh4g/oc-mirror-build
#Build image
podman build -t $IMG .
podman push $IMG