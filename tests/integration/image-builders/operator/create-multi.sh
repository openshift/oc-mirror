#!/bin/bash
# Builds multi-arch versions of the operator bundle, related image, and catalog images.
# Appends IMAGE_VERSION (UTC date + source commit) to every new image tag.
#
# New images:
#   quay.io/oc-mirror/oc-mirror-dev:foo-bundle-v0.3.1-multi-${IMAGE_VERSION}
#   quay.io/oc-mirror/oc-mirror-dev:foo-v0.3.1-multi-${IMAGE_VERSION}
#   quay.io/oc-mirror/oc-mirror-dev:bar-bundle-v1.0.0-multi-${IMAGE_VERSION}
#   quay.io/oc-mirror/oc-mirror-dev:bar-v1.0.0-multi-${IMAGE_VERSION}
#   quay.io/oc-mirror/oc-mirror-dev:test-catalog-multi-${IMAGE_VERSION}
#
# Prerequisites: podman (with --platform support via QEMU or native), cosign
# Run with make push-multi from the image-builders/operator/ directory.
#
# After a successful run:
#   make sign-multi IMAGE_VERSION=<the same version used here>

set -ex

REGISTRY="quay.io/oc-mirror/oc-mirror-dev"
IMAGE_VERSION="${IMAGE_VERSION:-$(date -u +%Y%m%d)-$(git rev-parse --short HEAD)}"
CATALOG_TAG="test-catalog-multi-${IMAGE_VERSION}"

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

for BASE_TAG in foo-bundle-v0.3.1 bar-bundle-v1.0.0; do
    BUNDLE_TAG="${BASE_TAG}-multi-${IMAGE_VERSION}"
    # Source directories retain the operator version, without the image revision.
    if [[ "${BASE_TAG}" == foo-* ]]; then
        SRC_DIR="bundles/foo/${BASE_TAG}"
    else
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

for BASE_TAG in foo-v0.3.1 bar-v1.0.0; do
    RELATED_TAG="${BASE_TAG}-multi-${IMAGE_VERSION}"
    BUNDLE_TAG="${BASE_TAG/-v/-bundle-v}-multi-${IMAGE_VERSION}"
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

# Update foo bundles.yaml — replace channel-head (v0.3.1) references
sed -E -i \
    -e "s|${REGISTRY}:foo-bundle-v0\.3\.1(-[[:alnum:]_.-]+)?\b|${REGISTRY}:foo-bundle-v0.3.1-multi-${IMAGE_VERSION}|g" \
    catalogs/test-catalog-multi/foo/bundles.yaml

# Update bar bundles.yaml — replace channel-head (v1.0.0) references
sed -E -i \
    -e "s|${REGISTRY}:bar-bundle-v1\.0\.0(-[[:alnum:]_.-]+)?\b|${REGISTRY}:bar-bundle-v1.0.0-multi-${IMAGE_VERSION}|g" \
    catalogs/test-catalog-multi/bar/bundles.yaml

# ─── Catalog image: test-catalog-multi ────────────────────────────────────────
# opm is arch-specific so we need per-arch builds.

for ARCH in amd64 arm64; do
    podman build --platform "linux/${ARCH}" \
        -f catalogs/catalog.Dockerfile \
        --build-arg "CATALOG=test-catalog-multi" \
        -t "${REGISTRY}:${CATALOG_TAG}-${ARCH}" \
        catalogs/
    podman push --remove-signatures "${REGISTRY}:${CATALOG_TAG}-${ARCH}"
done
push_manifest_list "${CATALOG_TAG}"

echo ""
echo "Successfully built and pushed all multi-arch operator images:"
echo "  ${REGISTRY}:foo-bundle-v0.3.1-multi-${IMAGE_VERSION}"
echo "  ${REGISTRY}:foo-v0.3.1-multi-${IMAGE_VERSION}"
echo "  ${REGISTRY}:bar-bundle-v1.0.0-multi-${IMAGE_VERSION}"
echo "  ${REGISTRY}:bar-v1.0.0-multi-${IMAGE_VERSION}"
echo "  ${REGISTRY}:${CATALOG_TAG}"
echo ""
echo "Next step:  make sign-multi IMAGE_VERSION=${IMAGE_VERSION}"
