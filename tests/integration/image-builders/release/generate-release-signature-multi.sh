#!/usr/bin/env bash
# Generate a throwaway GPG keypair, sign both the single-arch and multi-arch
# test release image digests (atomic container signature JSON),
# and write only the public key plus signatures under keys/.
# No registry or credentials required.
# Requires: gpg, jq. Paths are relative to the integration repo root.
#
# Both images are signed with the same key so that a single release-pk.asc
# validates all signatures. Run this script after building both release images.
# For single-arch only, use generate-release-signature.sh instead.
#
#      ./image-builders/release/generate-release-signature-multi.sh

set -euo pipefail

if [[ $# -gt 0 ]]; then
	echo "Usage: $0" >&2
	echo "  (no arguments — signs both releases, writes to keys/)" >&2
	exit 1
fi

command -v gpg >/dev/null || {
	echo "gpg is required" >&2
	exit 1
}
command -v jq >/dev/null || {
	echo "jq is required" >&2
	exit 1
}

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
KEYS_DIR="${REPO_ROOT}/keys"
GPG_IDENTITY="robot@test.com"

TMP_GNUPG="$(mktemp -d)"
cleanup() {
	rm -rf "${TMP_GNUPG}"
}
trap cleanup EXIT
export GNUPGHOME="${TMP_GNUPG}"
gpg --batch --passphrase '' --pinentry-mode loopback \
	--quick-generate-key "oc-mirror Release Test <${GPG_IDENTITY}>" rsa4096 default 0

mkdir -p "${KEYS_DIR}"
PK_OUT="${KEYS_DIR}/release-pk.asc"

sign_release() {
	local index_json="$1"
	local image_ref="$2"
	local use_file_digest="${3:-false}"

	if [[ ! -f "${index_json}" ]]; then
		echo "Skipping ${image_ref}: ${index_json} not found." >&2
		return 0
	fi

	local digest
	if [[ "${use_file_digest}" == "true" ]]; then
		# Multi-arch: the manifest list digest IS the sha256 of the index.json blob
		digest="sha256:$(sha256sum "${index_json}" | cut -d ' ' -f 1)"
	else
		digest="$(jq -r '
			([.manifests[] | select(.platform.os == "linux" and .platform.architecture == "amd64") | .digest][0])
			// .manifests[0].digest
		' "${index_json}")"
		if [[ -z "${digest}" || "${digest}" == "null" || "${digest}" != sha256:* ]]; then
			echo "Could not read a valid manifest digest from ${index_json}" >&2
			exit 1
		fi
	fi

	local digest_hex="${digest#sha256:}"
	local tag="${image_ref##*:}"

	local sig_payload
	sig_payload="$(jq -nc \
		--arg ref "${image_ref}" \
		--arg dig "${digest}" \
		'{critical:{identity:{"docker-reference":$ref},image:{"docker-manifest-digest":$dig},type:"atomic container signature"},optional:{}}')"

	local sig_file="${KEYS_DIR}/${tag}-sha256-${digest_hex}"

	rm -f "${sig_file}"
	printf '%s' "${sig_payload}" | gpg --batch \
		--output "${sig_file}" \
		--local-user "${GPG_IDENTITY}" \
		--digest-algo SHA512 \
		--sign

	if [[ ! -f "${sig_file}" ]]; then
		echo "Signing failed — ${sig_file} was not created" >&2
		exit 1
	fi

	echo "Wrote ${sig_file}"
}

# Single-arch release
sign_release \
	"${REPO_ROOT}/image-builders/release/release-payload/index.json" \
	"quay.io/oc-mirror/release/test-release-index:v0.0.1"

# Multi-arch release (the manifest list digest is the sha256 of the index.json blob itself)
sign_release \
	"${REPO_ROOT}/image-builders/release/release-payload-multi/index.json" \
	"quay.io/oc-mirror/release/test-release-index-multi:v0.0.1" \
	"true"

gpg -a --export --output "${PK_OUT}" "${GPG_IDENTITY}"
echo "Wrote ${PK_OUT}"
