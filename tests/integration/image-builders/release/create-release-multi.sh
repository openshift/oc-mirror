#!/bin/bash
# Builds a multi-arch fake OCP release payload OCI layout and pushes it as
# quay.io/oc-mirror/release/test-release-index-multi:v0.0.1
#
# Prerequisites: skopeo, jq, sha256sum, stat
# Run from the image-builders/release/ directory.
#
# After a successful run:
#   make sign-multi   (cosign signature)
#   ./generate-release-signature.sh  (GPG atomic container signature)
#   Commit: release-payload-multi/index.json and the new GPG signature in ../../testdata/keys/

set -ex

REPO="quay.io/oc-mirror/release/test-release-index-multi"
TAG="v0.0.1"

# Per-arch config blob filenames (sha256 of content, pre-committed)
declare -A ARCH_CONFIG_BLOB=(
  ["amd64"]="8366d414d18526e99f4781ed84a93b5b96ebe464d223e587cf7f705829b27a12"
  ["arm64"]="2adb492d1caf0f239c447de80519b6b431e3c9feff0470aa35a91c87b71feff2"
  ["s390x"]="dbf7b4a726c89654b4d389f1c999eef5d058f7ca27e1ae57c16d88c8939df7eb"
  ["ppc64le"]="87e0d3b6c43e0b8c022f3a5c4d5a82333ab51937563523acebac831bffdd9956"
)

rm -rf release-payload-multi/blobs/sha256
mkdir -p release-payload-multi/blobs/sha256 artifacts/staging

# Step 1: Resolve the multi-arch component image digest and update image-references
TMP_DIGEST=$(skopeo inspect --raw docker://quay.io/oc-mirror/release/test-image-multi:v0.0.1 | sha256sum | cut -d ' ' -f 1)
sed "s|quay.io/oc-mirror/release/test-image-multi:v0.0.1|quay.io/oc-mirror/release/test-image-multi@sha256:${TMP_DIGEST}|g" \
    artifacts/release-manifests/image-references-base-multi-component > artifacts/release-manifests/image-references-multi-component

# Step 2: Build the shared release-manifests layer blob using multi-arch component references
cd artifacts/staging
cp ../release-manifests/image-references-multi-component ../release-manifests/image-references
tar -czvf tmp_layer.tar.gz ../release-manifests/
LAYER_DIGEST=$(sha256sum tmp_layer.tar.gz | cut -d ' ' -f 1)
LAYER_SIZE=$(stat --printf="%s" tmp_layer.tar.gz)
cp tmp_layer.tar.gz ../../release-payload-multi/blobs/sha256/${LAYER_DIGEST}
cd ../../

# Step 2: For each arch, build an OCI image manifest referencing the shared layer
MANIFESTS_JSON=""

for ARCH in amd64 arm64 s390x ppc64le; do
  CONFIG_BLOB_NAME="${ARCH_CONFIG_BLOB[$ARCH]}"
  CONFIG_BLOB_PATH="artifacts/config/${CONFIG_BLOB_NAME}"
  CONFIG_SIZE=$(stat --printf="%s" "${CONFIG_BLOB_PATH}")

  # Copy config blob to payload blobs directory
  cp "${CONFIG_BLOB_PATH}" release-payload-multi/blobs/sha256/${CONFIG_BLOB_NAME}

  # Build per-arch OCI image manifest by substituting all four placeholders
  sed -e "s/CONFIG_DIGEST/${CONFIG_BLOB_NAME}/g" \
      -e "s/CONFIG_SIZE/${CONFIG_SIZE}/g" \
      -e "s/LAYER_DIGEST/${LAYER_DIGEST}/g" \
      -e "s/LAYER_SIZE/${LAYER_SIZE}/g" \
      artifacts/config/config-multi.json > /tmp/manifest_${ARCH}.json

  MANIFEST_DIGEST=$(sha256sum /tmp/manifest_${ARCH}.json | cut -d ' ' -f 1)
  MANIFEST_SIZE=$(stat --printf="%s" /tmp/manifest_${ARCH}.json)
  cp /tmp/manifest_${ARCH}.json release-payload-multi/blobs/sha256/${MANIFEST_DIGEST}

  # Accumulate manifest entry for OCI index
  ENTRY="{\"mediaType\":\"application/vnd.oci.image.manifest.v1+json\",\"digest\":\"sha256:${MANIFEST_DIGEST}\",\"size\":${MANIFEST_SIZE},\"platform\":{\"os\":\"linux\",\"architecture\":\"${ARCH}\"}}"
  if [ -z "${MANIFESTS_JSON}" ]; then
    MANIFESTS_JSON="${ENTRY}"
  else
    MANIFESTS_JSON="${MANIFESTS_JSON},${ENTRY}"
  fi
done

# Step 3: Push each per-arch image to the registry with an arch-specific tag,
# then assemble a manifest list using podman manifest.
for ARCH in amd64 arm64 s390x ppc64le; do
  CONFIG_BLOB_NAME="${ARCH_CONFIG_BLOB[$ARCH]}"
  ARCH_MANIFEST_FILE="/tmp/manifest_${ARCH}.json"
  ARCH_MANIFEST_DIGEST=$(sha256sum "${ARCH_MANIFEST_FILE}" | cut -d ' ' -f 1)
  ARCH_MANIFEST_SIZE=$(stat --printf="%s" "${ARCH_MANIFEST_FILE}")

  # Build a single-arch OCI layout for this arch
  ARCH_DIR="/tmp/oci_${ARCH}"
  rm -rf "${ARCH_DIR}"
  mkdir -p "${ARCH_DIR}/blobs/sha256"
  echo '{"imageLayoutVersion": "1.0.0"}' > "${ARCH_DIR}/oci-layout"

  cp release-payload-multi/blobs/sha256/${LAYER_DIGEST} "${ARCH_DIR}/blobs/sha256/"
  cp release-payload-multi/blobs/sha256/${CONFIG_BLOB_NAME} "${ARCH_DIR}/blobs/sha256/"
  cp "${ARCH_MANIFEST_FILE}" "${ARCH_DIR}/blobs/sha256/${ARCH_MANIFEST_DIGEST}"

  cat > "${ARCH_DIR}/index.json" <<EOF2
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.index.v1+json",
  "manifests": [
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:${ARCH_MANIFEST_DIGEST}",
      "size": ${ARCH_MANIFEST_SIZE}
    }
  ]
}
EOF2

  # Push the per-arch image with an arch-specific tag
  skopeo copy oci:${ARCH_DIR} docker://${REPO}:${TAG}-${ARCH}
done

# Step 4: Assemble the multi-arch manifest list using podman manifest
podman manifest rm ${REPO}:${TAG} 2>/dev/null || true
podman manifest create ${REPO}:${TAG}
for ARCH in amd64 arm64 s390x ppc64le; do
  podman manifest add --arch=${ARCH} --os=linux ${REPO}:${TAG} docker://${REPO}:${TAG}-${ARCH}
done

# Push the manifest list to the registry
podman manifest push --all --remove-signatures ${REPO}:${TAG} docker://${REPO}:${TAG}

# Save the manifest index for committing (used by generate-release-signature.sh)
skopeo inspect --raw docker://${REPO}:${TAG} > release-payload-multi/index.json

# Ensure oci-layout marker exists
cat > release-payload-multi/oci-layout <<'EOF'
{"imageLayoutVersion": "1.0.0"}
EOF

# Restore image-references to original
cp artifacts/release-manifests/image-references-base artifacts/release-manifests/image-references

# Clean up staging
rm -rf artifacts/staging/*

echo ""
echo "Successfully pushed ${REPO}:${TAG}"
echo ""
echo "Next steps:"
echo "  make sign-multi                        (cosign tag-based signature)"
echo "  ./generate-release-signature-multi.sh  (GPG atomic container signature)"
echo "  cp keys/* ../../testdata/keys/"
echo "  Commit: release-payload-multi/index.json, ../../testdata/keys/release-pk.asc, ../../testdata/keys/v0.0.1-sha256-*"
