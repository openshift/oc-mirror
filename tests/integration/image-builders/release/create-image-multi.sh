#!/bin/bash
# Builds a multi-arch fake release component image and pushes it as
# quay.io/oc-mirror/release/test-image-multi:v0.0.1
#
# Prerequisites: go, skopeo, podman, sha256sum, stat
# Run from the image-builders/release/ directory.
#
# After a successful run:
#   make sign-image-multi  (cosign tag-based signature)
#   Commit: release-payload-multi-component/index.json

set -ex

REPO="quay.io/oc-mirror/release/test-image-multi"
TAG="v0.0.1"

rm -rf release-payload-multi-component/blobs/sha256
mkdir -p release-payload-multi-component/blobs/sha256

STAGING="$(mktemp -d)"
cleanup() { rm -rf "${STAGING}"; }
trap cleanup EXIT

# Step 1: Cross-compile the simple-release binary for each arch
for ARCH in amd64 arm64; do
    GOARCH="${ARCH}" GOOS=linux CGO_ENABLED=0 \
        go build -mod=readonly \
        -gcflags=all="-l -B -wb=false" \
        -ldflags="-w -s" \
        -o "${STAGING}/simple-release-${ARCH}" \
        ./cmd/simple-release/
done

# Step 2: For each arch, build an OCI image layout and push with an arch-specific tag
for ARCH in amd64 arm64; do
    # Create layer tarball (uncompressed first for diff_id, then gzip)
    LAYER_TAR="${STAGING}/layer_${ARCH}.tar"
    LAYER_GZ="${STAGING}/layer_${ARCH}.tar.gz"
    tar -cf "${LAYER_TAR}" -C "${STAGING}" \
        --transform "s/simple-release-${ARCH}/simple-release/" \
        "simple-release-${ARCH}"
    DIFF_ID=$(sha256sum "${LAYER_TAR}" | cut -d ' ' -f 1)
    gzip -c "${LAYER_TAR}" > "${LAYER_GZ}"
    LAYER_DIGEST=$(sha256sum "${LAYER_GZ}" | cut -d ' ' -f 1)
    LAYER_SIZE=$(stat --printf="%s" "${LAYER_GZ}")
    cp "${LAYER_GZ}" release-payload-multi-component/blobs/sha256/${LAYER_DIGEST}

    # Create config blob
    CONFIG_FILE="${STAGING}/config_${ARCH}.json"
    cat > "${CONFIG_FILE}" <<EOF
{
  "created": "2023-01-24T02:27:38Z",
  "architecture": "${ARCH}",
  "os": "linux",
  "config": {
    "Entrypoint": ["/simple-release"],
    "Labels": {
      "io.openshift.release.component": "alpine"
    }
  },
  "rootfs": {
    "type": "layers",
    "diff_ids": ["sha256:${DIFF_ID}"]
  },
  "history": [
    {
      "created": "2023-01-24T02:27:38Z",
      "comment": "Test component image for oc-mirror v2 integration tests"
    }
  ]
}
EOF
    CONFIG_DIGEST=$(sha256sum "${CONFIG_FILE}" | cut -d ' ' -f 1)
    CONFIG_SIZE=$(stat --printf="%s" "${CONFIG_FILE}")
    cp "${CONFIG_FILE}" release-payload-multi-component/blobs/sha256/${CONFIG_DIGEST}

    # Create image manifest
    MANIFEST_FILE="${STAGING}/manifest_${ARCH}.json"
    cat > "${MANIFEST_FILE}" <<EOF
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "config": {
    "mediaType": "application/vnd.oci.image.config.v1+json",
    "digest": "sha256:${CONFIG_DIGEST}",
    "size": ${CONFIG_SIZE}
  },
  "layers": [
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
      "digest": "sha256:${LAYER_DIGEST}",
      "size": ${LAYER_SIZE}
    }
  ]
}
EOF
    MANIFEST_DIGEST=$(sha256sum "${MANIFEST_FILE}" | cut -d ' ' -f 1)
    MANIFEST_SIZE=$(stat --printf="%s" "${MANIFEST_FILE}")
    cp "${MANIFEST_FILE}" release-payload-multi-component/blobs/sha256/${MANIFEST_DIGEST}

    # Build a single-arch OCI layout for this arch
    ARCH_DIR="${STAGING}/oci_${ARCH}"
    mkdir -p "${ARCH_DIR}/blobs/sha256"
    echo '{"imageLayoutVersion": "1.0.0"}' > "${ARCH_DIR}/oci-layout"
    cp "${LAYER_GZ}" "${ARCH_DIR}/blobs/sha256/${LAYER_DIGEST}"
    cp "${CONFIG_FILE}" "${ARCH_DIR}/blobs/sha256/${CONFIG_DIGEST}"
    cp "${MANIFEST_FILE}" "${ARCH_DIR}/blobs/sha256/${MANIFEST_DIGEST}"
    cat > "${ARCH_DIR}/index.json" <<EOF2
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.index.v1+json",
  "manifests": [
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:${MANIFEST_DIGEST}",
      "size": ${MANIFEST_SIZE}
    }
  ]
}
EOF2

    # Push the per-arch image with an arch-specific tag
    skopeo copy oci:${ARCH_DIR} docker://${REPO}:${TAG}-${ARCH}
done

# Step 3: Assemble the multi-arch manifest list using podman manifest
podman manifest rm ${REPO}:${TAG} 2>/dev/null || true
podman manifest create ${REPO}:${TAG}
for ARCH in amd64 arm64; do
    podman manifest add --arch=${ARCH} --os=linux ${REPO}:${TAG} docker://${REPO}:${TAG}-${ARCH}
done
podman manifest push --all --remove-signatures ${REPO}:${TAG} docker://${REPO}:${TAG}

# Save the manifest index for committing
mkdir -p release-payload-multi-component
skopeo inspect --raw docker://${REPO}:${TAG} > release-payload-multi-component/index.json
cat > release-payload-multi-component/oci-layout <<'EOF'
{"imageLayoutVersion": "1.0.0"}
EOF

echo ""
echo "Successfully pushed ${REPO}:${TAG}"
echo ""
echo "Next steps:"
echo "  make sign-image-multi  (cosign tag-based signature)"
echo "  Commit: release-payload-multi-component/index.json"
