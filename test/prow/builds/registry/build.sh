#!/bin/sh
set -e
#Prepare
IMG=quay.io/samwalke/oc-mirror-registry
REGISTRY="localhost:5001"
sudo mkdir /mnt/registry -p
sudo chmod 777 /mnt/registry

#Setup registry
podman run -p 5001:5000 -v /mnt/registry:/var/lib/registry registry:2 &
#Run partial test data setup
mkdir -p /tmp/head/create
test/operator/setup-testdata.sh /tmp/head /tmp/head/create "latest/imageset-config-full.yaml" false
#stop
podman stop --all
pushd test/prow/builds/registry
mv /mnt/registry/* registry
#Build image
podman build -t $IMG .
podman push $IMG

#cleanup 
sudo rm -rf /mnt/registry 
sudo rm -rf registry/*
#rm -rf /tmp/head
popd

