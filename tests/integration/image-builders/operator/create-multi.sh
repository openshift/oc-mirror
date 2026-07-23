#!/bin/bash
# Builds multi-arch versions of the operator bundle, related image, and catalog images.
# Creates new *-multi tagged images alongside the existing single-arch images.
#
# New images:
#   quay.io/oc-mirror/oc-mirror-dev:foo-bundle-v0.3.1-multi  (amd64 + arm64)
#   quay.io/oc-mirror/oc-mirror-dev:foo-v0.3.1-multi         (amd64 + arm64)
#   quay.io/oc-mirror/oc-mirror-dev:bar-bundle-v1.0.0-multi  (amd64 + arm64)
#   quay.io/oc-mirror/oc-mirror-dev:bar-v1.0.0-multi         (amd64 + arm64)
#   quay.io/oc-mirror/oc-mirror-dev:test-catalog-multi       (amd64 + arm64)
#
# Prerequisites: podman (with --platform support via QEMU or native), skopeo, cosign
# Run from the image-builders/operator/ directory.
#
# After a successful run:
#   make sign-multi

set -ex

REGISTRY="quay.io/oc-mirror/oc-mirror-dev"

push_manifest_list() {
    local TAG="${1}"
    podman manifest rm "${REGISTRY}:${TAG}" 2>/dev/null || true
    podman manifest create "${REGISTRY}:${TAG}"
    for ARCH in amd64 arm64; do
        podman manifest add --arch="${ARCH}" --os=linux \
            "${REGISTRY}:${TAG}" "docker://${REGISTRY}:${TAG}-${ARCH}"
    done
    podman manifest push --all --remove-signatures \
        "${REGISTRY}:${TAG}" "docker://${REGISTRY}:${TAG}"
}

# ─── Bundle images (FROM scratch — platform is only in the config blob) ────────

for BUNDLE_TAG in foo-bundle-v0.3.1-multi bar-bundle-v1.0.0-multi; do
    # Determine source Dockerfile path from tag
    if [[ "${BUNDLE_TAG}" == foo-* ]]; then
        BASE_TAG="${BUNDLE_TAG%-multi}"     # foo-bundle-v0.3.1
        SRC_DIR="bundles/foo/${BASE_TAG}"
    else
        BASE_TAG="${BUNDLE_TAG%-multi}"     # bar-bundle-v1.0.0
        SRC_DIR="bundles/bar/${BASE_TAG}"
    fi

    for ARCH in amd64 arm64; do
        podman build --platform "linux/${ARCH}" \
            -t "${REGISTRY}:${BUNDLE_TAG}-${ARCH}" \
            "${SRC_DIR}/"
        podman push --remove-signatures "${REGISTRY}:${BUNDLE_TAG}-${ARCH}"
    done
    push_manifest_list "${BUNDLE_TAG}"
done

# ─── Related images ────────────────────────────────────────────────────────────
# The related_image/Dockerfile uses a pinned single-arch distroless digest.
# Build with --platform using the manifest list digest so each arch gets the right base.

DISTROLESS_ML="gcr.io/distroless/static@sha256:9197324ba51d9cd071af8505989365c006adf9d6d2067eada25aef00abbb5278"

for RELATED_TAG in foo-v0.3.1-multi bar-v1.0.0-multi; do
    BUNDLE_TAG="${RELATED_TAG/v0.3.1-multi/bundle-v0.3.1-multi}"
    BUNDLE_TAG="${BUNDLE_TAG/v1.0.0-multi/bundle-v1.0.0-multi}"
    BUNDLE_FULL="${REGISTRY}:${BUNDLE_TAG}"

    for ARCH in amd64 arm64; do
        podman build --platform "linux/${ARCH}" \
            --build-arg "RELATED_IMAGE=${BUNDLE_FULL}" \
            --build-arg "DISTROLESS_IMAGE=${DISTROLESS_ML}" \
            -t "${REGISTRY}:${RELATED_TAG}-${ARCH}" \
            --file - related_image/ <<'EOF'
ARG DISTROLESS_IMAGE=gcr.io/distroless/static:latest
FROM ${DISTROLESS_IMAGE}
ARG RELATED_IMAGE
COPY run.sh /
ENV RELATED_IMAGE=${RELATED_IMAGE}
ENTRYPOINT ["/run.sh"]
EOF
        podman push --remove-signatures "${REGISTRY}:${RELATED_TAG}-${ARCH}"
    done
    push_manifest_list "${RELATED_TAG}"
done

# ─── Catalog FBC: test-catalog-multi ──────────────────────────────────────────
# Copy test-catalog-latest FBC and update the channel-head bundle references.

rm -rf catalogs/test-catalog-multi
cp -r catalogs/test-catalog-latest catalogs/test-catalog-multi

# Get manifest list digests for the new bundles (for relatedImages in catalog FBC)
FOO_BUNDLE_DIGEST=$(skopeo inspect --raw "docker://${REGISTRY}:foo-bundle-v0.3.1-multi" | sha256sum | cut -d ' ' -f 1)
BAR_BUNDLE_DIGEST=$(skopeo inspect --raw "docker://${REGISTRY}:bar-bundle-v1.0.0-multi" | sha256sum | cut -d ' ' -f 1)
FOO_RELATED_DIGEST=$(skopeo inspect --raw "docker://${REGISTRY}:foo-v0.3.1-multi" | sha256sum | cut -d ' ' -f 1)
BAR_RELATED_DIGEST=$(skopeo inspect --raw "docker://${REGISTRY}:bar-v1.0.0-multi" | sha256sum | cut -d ' ' -f 1)

# Update foo bundles.yaml — replace channel-head (v0.3.1) references
sed -i \
    -e "s|${REGISTRY}:foo-bundle-v0.3.1\b|${REGISTRY}:foo-bundle-v0.3.1-multi|g" \
    catalogs/test-catalog-multi/foo/bundles.yaml

# Update bar bundles.yaml — replace channel-head (v1.0.0) references
sed -i \
    -e "s|${REGISTRY}:bar-bundle-v1.0.0\b|${REGISTRY}:bar-bundle-v1.0.0-multi|g" \
    catalogs/test-catalog-multi/bar/bundles.yaml

# ─── Catalog image: test-catalog-multi ────────────────────────────────────────
# opm is arch-specific so we need per-arch builds.

for ARCH in amd64 arm64; do
    podman build --platform "linux/${ARCH}" \
        -f catalogs/catalog.Dockerfile \
        --build-arg "CATALOG=test-catalog-multi" \
        -t "${REGISTRY}:test-catalog-multi-${ARCH}" \
        catalogs/
    podman push --remove-signatures "${REGISTRY}:test-catalog-multi-${ARCH}"
done
push_manifest_list "test-catalog-multi"

echo ""
echo "Successfully built and pushed all multi-arch operator images:"
echo "  ${REGISTRY}:foo-bundle-v0.3.1-multi"
echo "  ${REGISTRY}:foo-v0.3.1-multi"
echo "  ${REGISTRY}:bar-bundle-v1.0.0-multi"
echo "  ${REGISTRY}:bar-v1.0.0-multi"
echo "  ${REGISTRY}:test-catalog-multi"
echo ""
echo "Next step:  make sign-multi"
