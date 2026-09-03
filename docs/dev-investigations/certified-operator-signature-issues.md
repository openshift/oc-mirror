# Certified Operator Index: Container Image Signature Verification Failures

## Table of contents
- [Background](#background)
- [Problem statement](#problem-statement)
- [Reproducing the issue](#reproducing-the-issue)
- [Root cause analysis](#root-cause-analysis)
  - [The signature lookup mechanism](#the-signature-lookup-mechanism)
  - [HTTP response codes: the key distinction oc-mirror ignores](#http-response-codes-the-key-distinction-oc-mirror-ignores)
  - [The error chain in code](#the-error-chain-in-code)
  - [Cascade failure: operator bundles skipped](#cascade-failure-operator-bundles-skipped)
  - [Existing escape hatch and why it is insufficient](#existing-escape-hatch-and-why-it-is-insufficient)
- [Implemented fix: --signature-verification flag](#implemented-fix---signature-verification-flag)
  - [Flag definition](#flag-definition)
  - [Behavior matrix](#behavior-matrix)
  - [How the retry logic works](#how-the-retry-logic-works)
  - [Error classification: signed-but-unreachable vs unsigned](#error-classification-signed-but-unreachable-vs-unsigned)
  - [Archive blob gathering tolerance](#archive-blob-gathering-tolerance)
- [Validation](#validation)
- [Files modified](#files-modified)

## Background

oc-mirror v2 enforces container image signature verification by default. When copying
images, it configures the `go.podman.io/image/v5` library (the upstream
`containers/image` library) to look for cosign/sigstore signature attachments stored as
OCI tags in the source registry. If the signature tag exists, it is read from the source
and written to the destination alongside the image. If the signature tag does not exist,
the library treats this as a fatal copy error.

The Red Hat certified operator index (`registry.redhat.io/redhat/certified-operator-index`)
contains operators from ISVs (Independent Software Vendors) whose container images are
published to `registry.connect.redhat.com`. These ISV images are **not required to be
signed** as part of the Red Hat certified operator program. Meanwhile, the related images
that originate from Red Hat itself (e.g., CSI sidecars from `registry.redhat.io/openshift4/`)
**are signed**.

This creates a mixed environment where some images in a single operator bundle have
valid signatures and others do not. oc-mirror's signature handling has no concept of
this distinction.

## Problem statement

oc-mirror cannot distinguish between:

1. **An image that was never signed** -- the signature tag was never created in the
   registry because the publisher did not sign the image. This is expected and legitimate
   for certified operator images.

2. **A signature that exists but could not be retrieved** -- a transient or permanent
   error (network, authentication, server failure) prevented fetching a real signature.

Both cases produce the same fatal error from the upstream `containers/image` library,
causing the image copy to fail, which cascades to skip the entire operator bundle.

## Reproducing the issue

**ImageSetConfiguration** (`imageset-config.yaml`):

```yaml
apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
mirror:
  operators:
    - catalog: registry.redhat.io/redhat/certified-operator-index:v4.19
      packages:
        - name: trident-operator
          channels:
            - name: stable
              minVersion: 26.2.0
              maxVersion: 26.2.0
```

**Command** (prior to fix):

```bash
oc-mirror --v2 --config ./imageset-config.yaml file:///path/to/mirror
```

**Result**: 7 of 11 operator images mirror successfully. 4 fail (3 related images + 1
operator bundle skipped due to the cascade).

**Successful images** (all from `registry.redhat.io`, Red Hat-signed):
- `openshift4/ose-csi-external-provisioner-rhel9`
- `openshift4/ose-csi-external-attacher-rhel9`
- `openshift4/ose-csi-external-snapshotter-rhel9`
- `openshift4/ose-csi-node-driver-registrar-rhel9`
- `openshift4/ose-csi-livenessprobe-rhel9`
- `openshift4/ose-csi-external-resizer-rhel9`
- `redhat/certified-operator-index:v4.19` (the catalog itself)

**Failed images** (all from `registry.connect.redhat.com`, ISV unsigned):
- `netapp/trident-autosupport@sha256:566667b5...`
- `netapp/trident@sha256:c40d71da...`
- `netapp/trident-operator-image@sha256:fba8d9ed...`
- `netapp/trident-operator@sha256:9fc86f24...` (bundle -- skipped due to cascade)

## Root cause analysis

### The signature lookup mechanism

oc-mirror's signature handling works through a chain of configuration:

1. **registries.d config generation** (`internal/pkg/registriesd/registriesd.go`):
   `PrepareRegistrydCustomDir()` creates YAML config files with
   `use-sigstore-attachments: true` for every source registry encountered during
   mirroring. These are written to the working directory under
   `<working-dir>/containers/registries.d/`.

2. **SystemContext configuration** (`internal/pkg/mirror/mirror.go`):
   When `RemoveSignatures` is `false`, the `RegistriesDirPath` on both the source and
   destination `SystemContext` is set to point at the custom registries.d directory.

3. **Signature tag construction**: The upstream library converts an image digest to a
   signature tag by replacing the `:` in `sha256:<digest>` with `-` and appending `.sig`:
   ```
   sha256:566667b5e321b6b3b067e704b8a84f6bfce0425866ccec69674e45a6d4c2b197
   -->
   sha256-566667b5e321b6b3b067e704b8a84f6bfce0425866ccec69674e45a6d4c2b197.sig
   ```

4. **Signature fetch**: The library issues a `GET` request against the source registry for
   the `.sig` tag manifest. If the response is not `200 OK`, it returns an error.

### HTTP response codes: the key distinction oc-mirror ignores

The registry's HTTP response when oc-mirror queries for a `.sig` tag provides exactly
the information needed to distinguish unsigned images from retrieval failures:

**Signed images (Red Hat, `registry.redhat.io`)** -- the `.sig` tag exists:

```
GET /v2/openshift4/ose-csi-external-attacher-rhel9/manifests/sha256-8c7a20b3...22.sig  -->  200  (24349 bytes)
PUT /v2/openshift4/ose-csi-external-attacher-rhel9/manifests/sha256-8c7a20b3...22.sig  -->  201
```

The source registry returns **HTTP 200** with the signature manifest (typically ~24KB).
oc-mirror then writes it to the local cache with a **PUT** that returns **HTTP 201**.

**Unsigned images (ISV, `registry.connect.redhat.com`)** -- the `.sig` tag was never
created:

```
GET /v2/netapp/trident-autosupport/manifests/sha256-566667b5...97.sig  -->  404  (169 bytes)
```

The source registry returns **HTTP 404** with a short JSON error body. The registry's
error response is `"name unknown: Image not found"`, indicating the repository path
for the `.sig` tag does not exist. There is no `PUT` because the `GET` already failed.

**Retrieval failures** (network/auth/server errors) would produce different responses:

| HTTP Code | Meaning | Should be an error? |
|-----------|---------|---------------------|
| `200` | Signature exists and was retrieved | No (success) |
| `401` | Authentication failure | Yes -- signature may exist |
| `403` | Authorization denied | Yes -- signature may exist |
| `404` | Tag/manifest does not exist | No -- image was never signed |
| `500-504` | Server error | Yes -- transient failure |
| Network timeout | Connection problem | Yes -- transient failure |

The upstream library wraps the HTTP 404 into an error string:

```
reading signatures: reading manifest sha256-566667b5...97.sig
  in registry.connect.redhat.com/netapp/trident-autosupport: name unknown: Image not found
```

This error string contains `"reading signatures"` combined with `"name unknown"`, which
is the distinctive pattern for an unsigned image (HTTP 404). A real retrieval failure
would contain different error text (timeout, connection refused, unauthorized, server
error, etc.).

### The error chain in code

```
PrepareRegistrydCustomDir()                    // registriesd.go - writes use-sigstore-attachments: true
  |-> sourceCtx.RegistriesDirPath = ...        // mirror.go - tells library where to find the config
       |-> copy.Image()                        // upstream go.podman.io/image/v5 - reads config, fetches .sig tag
            |-> GET .../sha256-DIGEST.sig      // HTTP 404 from registry.connect.redhat.com
                 |-> "reading signatures: reading manifest ... name unknown: Image not found"
                      |-> err returned to batch worker
                           |-> errArray = append(errArray, *err)      // concurrent_chan_worker.go
                                |-> shouldSkipImage() matches bundle  // concurrent_chan_worker.go
                                     |-> bundle skipped
```

### Cascade failure: operator bundles skipped

When a related image fails to mirror, the batch worker records both the failing image
and the operator bundles it belongs to (`mirrorErrorSchema.bundles`). When the worker
later encounters the operator bundle image itself, `shouldSkipImage()` checks whether
any of the bundle's related images have already failed. If so, the bundle is skipped
with the error:

```
skipping operator bundle docker://registry.connect.redhat.com/netapp/trident-operator@sha256:9fc86f24...
  because one of its related images failed to mirror
```

This means a single unsigned related image causes the entire operator to fail.

### Existing escape hatch and why it is insufficient

The `--remove-signatures` flag prevents setting `RegistriesDirPath` on the
SystemContext AND sets `RemoveSignatures: true` on the copy options. This avoids the
signature read error entirely.

However, this is too blunt:

- It strips **all** signatures, including valid ones from Red Hat images
- It prevents signatures from being archived and transferred to the mirror registry
- Users in disconnected environments lose the ability to verify image provenance
- It is a binary choice: all signatures or no signatures

## Implemented fix: --signature-verification flag

### Flag definition

A new CLI flag `--signature-verification` accepts two string values:

```
--signature-verification string   Signature verification mode:
                                  "strict" requires all images to have signatures;
                                  "best-effort" skips missing signatures for unsigned images
                                  (default "strict")
```

Registered in `internal/pkg/cli/executor.go` alongside the existing signature flags.
Stored as `GlobalOptions.SignatureVerification` in `internal/pkg/mirror/options.go`.

### Behavior matrix

| `--remove-signatures` | `--signature-verification` | Behavior |
|---|---|---|
| `false` | `strict` (default) | Current behavior: copy signatures, **fail on missing** |
| `false` | `best-effort` | **New**: copy signatures when available, **skip gracefully when absent** |
| `true` | (either) | `--remove-signatures` wins: no signatures read or copied |

### How the retry logic works

The fix is implemented in `internal/pkg/mirror/mirror.go` inside the `copy()` function.
When `copy.Image()` fails, three conditions are checked:

1. `RemoveSignatures` is `false` (signatures are being processed)
2. `SignatureVerification` is `"best-effort"`
3. The error matches the "signature not found" pattern

If all three are true, a retry copy is attempted with:
- `RemoveSignatures` set to `true`
- `RegistriesDirPath` cleared on both source and destination contexts

This retry tells the upstream library to skip signature handling entirely for **just
that image**. Signed images never trigger this path because their first copy attempt
succeeds.

```go
manifestBytes, err := o.mc.CopyImage(ctx, policyContext, destRef, srcRef, co)
if err != nil && !opts.RemoveSignatures &&
    opts.SrcImage.global.SignatureVerification == SignatureVerificationBestEffort &&
    IsSignatureNotFoundError(err) {
    // The image is unsigned - retry without signature reading
    coRetry := *co
    coRetry.RemoveSignatures = true
    sourceCtxRetry := *co.SourceCtx
    sourceCtxRetry.RegistriesDirPath = ""
    coRetry.SourceCtx = &sourceCtxRetry
    destCtxRetry := *co.DestinationCtx
    destCtxRetry.RegistriesDirPath = ""
    coRetry.DestinationCtx = &destCtxRetry
    manifestBytes, err = o.mc.CopyImage(ctx, policyContext, destRef, srcRef, &coRetry)
}
```

### Error classification: signed-but-unreachable vs unsigned

The `IsSignatureNotFoundError()` function (`internal/pkg/mirror/mirror.go`) classifies
errors by inspecting the error message text:

```go
func IsSignatureNotFoundError(err error) bool {
    if err == nil {
        return false
    }
    msg := err.Error()
    return strings.Contains(msg, "reading signatures") &&
        (strings.Contains(msg, "name unknown") || strings.Contains(msg, "manifest unknown"))
}
```

This matches the specific error patterns produced by the upstream library when the
registry responds with HTTP 404:

| Error text | Registry HTTP response | Meaning | Matched? |
|---|---|---|---|
| `reading signatures: ... name unknown: Image not found` | 404 | Image was never signed | Yes -- retry |
| `reading signatures: ... manifest unknown` | 404 | Signature manifest does not exist | Yes -- retry |
| `reading signatures: ... unauthorized` | 401 | Auth failure, signature may exist | No -- error |
| `reading signatures: ... connection refused` | N/A | Network failure | No -- error |
| `reading signatures: ... 500 Internal Server Error` | 500 | Server failure | No -- error |
| (no `reading signatures` prefix) | varies | Non-signature error | No -- error |

Errors that do **not** match the pattern are returned as-is, preserving the strict
failure behavior for genuine retrieval problems.

### Archive blob gathering tolerance

The same distinction is applied during the archive phase. When oc-mirror gathers blobs
for the tar archive, it attempts to read signature manifests from the local cache
(`internal/pkg/archive/image-blob-gatherer.go:imageSignatureBlobs()`).

For unsigned images in best-effort mode, the signature manifest will not exist in the
cache (since it was never fetched). The gatherer checks for the same error pattern and
returns `nil, nil` (no blobs, no error) instead of a `SignatureBlobGathererError`,
preventing the archive phase from failing.

## Validation

**Before fix** (strict mode, default):
```
7 / 11 operator images mirrored: Some operator images failed to be mirrored
```

**After fix** (`--signature-verification best-effort`):
```
11 / 11 operator images mirrored successfully
```

In the successful run, the log shows:
- Red Hat images: `.sig` tags fetched with **HTTP 200**, written to cache with **HTTP 201**
- NetApp ISV images: `.sig` tags return **HTTP 404**, archive phase logs
  `Skip signature gathering for manifest list "sha256:..."` at DEBUG level, and the
  images are mirrored without signatures
- The certified-operator-index catalog image itself: `.sig` tags return **HTTP 404**
  (the rebuilt catalog is not signed), handled gracefully

**Usage**:

```bash
oc-mirror --v2 --config ./imageset-config.yaml \
  --signature-verification best-effort \
  file:///path/to/mirror
```

## Files modified

| File | Change |
|------|--------|
| `internal/pkg/mirror/const.go` | Added `SignatureVerificationStrict` and `SignatureVerificationBestEffort` constants |
| `internal/pkg/mirror/options.go` | Added `SignatureVerification string` field to `GlobalOptions` |
| `internal/pkg/mirror/mirror.go` | Added `IsSignatureNotFoundError()` classifier and retry-without-signatures logic in `copy()` |
| `internal/pkg/cli/executor.go` | Registered `--signature-verification` flag (default `"strict"`), added input validation |
| `internal/pkg/archive/image-blob-gatherer.go` | Return `nil, nil` instead of error when signature manifest is absent in best-effort mode |
| `internal/pkg/cli/executor_test.go` | Set `SignatureVerification` default in test fixtures for `TestExecutorValidate` |
