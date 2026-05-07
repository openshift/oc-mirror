#!/usr/bin/env bash
# Generate a throwaway GPG keypair, sign the test release image digest from
# images/release/release-payload/index.json (atomic container signature JSON),
# and write only the public key plus signature under keys/. No registry or credentials.
# Requires: gpg, jq. Paths are relative to the integration repo root.
#
#      ./images/release/generate-release-signature.sh

set -euo pipefail

if [[ $# -gt 0 ]]; then
	echo "Usage: $0" >&2
	echo "  (no arguments — reads images/release/release-payload/index.json, writes keys/)" >&2
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
INDEX_JSON="${REPO_ROOT}/images/release/release-payload/index.json"
KEYS_DIR="${REPO_ROOT}/keys"
IMAGE_REF="quay.io/oc-mirror/release/test-release-index:v0.0.1"
GPG_IDENTITY="robot@test.com"

if [[ ! -f "${INDEX_JSON}" ]]; then
	echo "Missing ${INDEX_JSON} — build the OCI release layout first so index.json exists." >&2
	exit 1
fi

DIGEST="$(jq -r '
	([.manifests[] | select(.platform.os == "linux" and .platform.architecture == "amd64") | .digest][0])
	// .manifests[0].digest
' "${INDEX_JSON}")"
if [[ -z "${DIGEST}" || "${DIGEST}" == "null" ]]; then
	echo "Could not read a manifest digest from ${INDEX_JSON}" >&2
	exit 1
fi
if [[ "${DIGEST}" != sha256:* ]]; then
	echo "Expected sha256 digest, got: ${DIGEST}" >&2
	exit 1
fi

DIGEST_HEX="${DIGEST#sha256:}"
TAG="${IMAGE_REF##*:}"

SIG_PAYLOAD="$(jq -nc \
	--arg ref "${IMAGE_REF}" \
	--arg dig "${DIGEST}" \
	'{critical:{identity:{"docker-reference":$ref},image:{"docker-manifest-digest":$dig},type:"atomic container signature"},optional:{}}')"

TMP_GNUPG="$(mktemp -d)"
cleanup() {
	rm -rf "${TMP_GNUPG}"
}
trap cleanup EXIT
export GNUPGHOME="${TMP_GNUPG}"

gpg --batch --passphrase '' --pinentry-mode loopback \
	--quick-generate-key "oc-mirror Release Test <${GPG_IDENTITY}>" rsa4096 default 0

mkdir -p "${KEYS_DIR}"
SIG_FILE="${KEYS_DIR}/${TAG}-sha256-${DIGEST_HEX}"
PK_OUT="${KEYS_DIR}/release-pk.asc"

printf '%s' "${SIG_PAYLOAD}" | gpg --batch \
	--output "${SIG_FILE}" \
	--local-user "${GPG_IDENTITY}" \
	--digest-algo SHA512 \
	--sign

if [[ ! -f "${SIG_FILE}" ]]; then
	echo "Signing failed — ${SIG_FILE} was not created" >&2
	exit 1
fi

gpg -a --export --output "${PK_OUT}" "${GPG_IDENTITY}"

echo "Wrote ${PK_OUT}"
echo "Wrote ${SIG_FILE}"
