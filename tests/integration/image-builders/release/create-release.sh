#!/bin/bash

set -ex

make build

make container 

make push 

TMP_DIGEST=$(podman inspect quay.io/oc-mirror/release/test-image:v0.0.1 | jq '.[].Digest')
DIGEST=$(echo "${TMP_DIGEST}" | tr -d '"')
sed "s/new-sha/${DIGEST}/g" artifacts/release-manifests/image-references-base > artifacts/release-manifests/image-references

rm -rf release-payload/blobs/sha256/*

mkdir -p artifacts/staging

cd artifacts/staging

tar -czvf tmp_file.tar.gz ../release-manifests/

ART_DIGEST=$(sha256sum tmp_file.tar.gz | cut -d ' ' -f 1)

cp tmp_file.tar.gz ${ART_DIGEST}

cp ${ART_DIGEST} ../../release-payload/blobs/sha256/

FILE_SIZE=$(stat --printf="%s" ${ART_DIGEST})

sed "s/SIZE/${FILE_SIZE}/g" ../config/config.json > ../config/tmp.json
sed "s/UPDATE_DIGEST/${ART_DIGEST}/g" ../config/tmp.json > ../config/final.json

CONFIG_DIGEST=$(sha256sum ../config/final.json | cut -d ' ' -f 1)

cp ../config/final.json ../config/${CONFIG_DIGEST}
CONFIG_SIZE=$(stat --printf="%s" ../config/${CONFIG_DIGEST})
cp ../config/${CONFIG_DIGEST} ../../release-payload/blobs/sha256/
rm -rf ../config/${CONFIG_DIGEST}
rm -rf ../config/final.json ../config/tmp.json

sed "s/SIZE/${CONFIG_SIZE}/g" ../config/index.json > ../config/tmp_index.json
sed "s/UPDATE_DIGEST/${CONFIG_DIGEST}/g" ../config/tmp_index.json > ../config/final_index.json

cp ../config/8366d414d18526e99f4781ed84a93b5b96ebe464d223e587cf7f705829b27a12 ../../release-payload/blobs/sha256/
cp ../config/final_index.json ../../release-payload/index.json
rm -rf ../config/tmp_index.json ../config/final_index.json
cd ../../
rm -rf artifacts/staging/*


