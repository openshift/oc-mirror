# Catalogs

Mock catalogs to be used in tests.

## test-catalog-latest

### Contents
 * Packages: foo, bar, baz
 * Channels:
    - foo: beta
    - bar: alpha, stable
    - baz: stable
 * Bundles:
    - foo: v0.1.0, v0.2.0, v0.3.0, v0.3.1
    - bar: v0.1.0, v0.2.0, v1.0.0
    - baz: v1.0.0, v1.0.1, v1.1.0

### Creating
```bash
CATALOG=test-catalog-latest
mkdir -p ${CATALOG}/{foo,bar,baz}

opm init foo -c beta -o yaml > ${CATALOG}/foo/operator.yaml
opm init bar -c stable -o yaml > ${CATALOG}/bar/operator.yaml
opm init baz -c stable -o yaml > ${CATALOG}/baz/operator.yaml

REPO="quay.io/oc-mirror/oc-mirror-dev"
opm render ${REPO}:foo-bundle-v0.1.0 ${REPO}:foo-bundle-v0.2.0 ${REPO}:foo-bundle-v0.3.0 ${REPO}:foo-bundle-v0.3.1 --output=yaml > ${CATALOG}/foo/bundles.yaml
opm render ${REPO}:bar-bundle-v0.1.0 ${REPO}:bar-bundle-v0.2.0 ${REPO}:bar-bundle-v1.0.0 --output=yaml > ${CATALOG}/bar/bundles.yaml
opm render ${REPO}:baz-bundle-v1.0.0 ${REPO}:baz-bundle-v1.0.1 ${REPO}:baz-bundle-v1.1.0 --output=yaml > ${CATALOG}/baz/bundles.yaml
```


## test-catalog-diff

### Contents
 * Packages: foo, bar, baz
 * Channels:
    - foo: beta
    - bar: alpha, stable
    - baz: stable
 * Bundles:
    - foo: v0.1.0, v0.2.0, v0.3.0, v0.3.1, v0.3.2
    - bar: v0.1.0, v0.2.0, v1.0.0
    - baz: v1.0.0, v1.0.1, v1.1.0

### Creating
```bash
CATALOG=test-catalog-diff
mkdir -p ${CATALOG}/{foo,bar,baz}

opm init foo -c beta -o yaml > ${CATALOG}/foo/operator.yaml
opm init bar -c stable -o yaml > ${CATALOG}/bar/operator.yaml
opm init baz -c stable -o yaml > ${CATALOG}/baz/operator.yaml

REPO="quay.io/oc-mirror/oc-mirror-dev"
opm render ${REPO}:foo-bundle-v0.1.0 ${REPO}:foo-bundle-v0.2.0 ${REPO}:foo-bundle-v0.3.0 ${REPO}:foo-bundle-v0.3.1 ${REPO}:foo-bundle-v0.3.2 --output=yaml > ${CATALOG}/foo/bundles.yaml
opm render ${REPO}:bar-bundle-v0.1.0 ${REPO}:bar-bundle-v0.2.0 ${REPO}:bar-bundle-v1.0.0 --output=yaml > ${CATALOG}/bar/bundles.yaml
opm render ${REPO}:baz-bundle-v1.0.0 ${REPO}:baz-bundle-v1.0.1 ${REPO}:baz-bundle-v1.1.0 --output=yaml > ${CATALOG}/baz/bundles.yaml
```


## test-catalog-prune

### Contents
 * Packages: foo, bar
 * Channels:
    - foo: beta
    - bar: alpha
 * Bundles:
    - foo: v0.1.0, v0.1.1
    - bar: v0.1.0

### Creating
```bash
CATALOG=test-catalog-prune
mkdir -p ${CATALOG}/{foo,bar}

opm init foo -c beta -o yaml > ${CATALOG}/foo/operator.yaml
opm init bar -c alpha -o yaml > ${CATALOG}/bar/operator.yaml

REPO="quay.io/oc-mirror/oc-mirror-dev"
opm render ${REPO}:foo-bundle-v0.1.0 ${REPO}:foo-bundle-v0.1.1 --output=yaml > ${CATALOG}/foo/bundles.yaml
opm render ${REPO}:bar-bundle-v0.1.0 --output=yaml > ${CATALOG}/bar/bundles.yaml
```


## test-catalog-prune-diff

### Contents
 * Packages: foo, bar
 * Channels:
    - foo: beta
    - bar: alpha, stable
 * Bundles:
    - foo: v0.2.0
    - bar: v0.1.0

### Creating
```bash
CATALOG=test-catalog-prune-diff
mkdir -p ${CATALOG}/{foo,bar}

opm init foo -c beta -o yaml > ${CATALOG}/foo/operator.yaml
opm init bar -c alpha -o yaml > ${CATALOG}/bar/operator.yaml

REPO="quay.io/oc-mirror/oc-mirror-dev"
opm render ${REPO}:foo-bundle-v0.2.0 --output=yaml > ${CATALOG}/foo/bundles.yaml
opm render ${REPO}:bar-bundle-v0.1.0 --output=yaml > ${CATALOG}/bar/bundles.yaml
```


## test-catalog-extra-blobs

This catalog contains blobs of types other than `Package`, `Channel`, and,
`Bundle`. These extra blobs were manually created. Even though some of them
might resemble existing blob types, their content is arbitrary and should not
be relied on.

### Contents
 * Packages: foo, bar, baz
 * Channels:
    - foo: beta
    - bar: alpha, stable
    - baz: stable
 * Bundles:
    - foo: v0.1.0, v0.2.0, v0.3.0, v0.3.1
    - bar: v0.1.0, v0.2.0, v1.0.0
    - baz: v1.0.0, v1.0.1, v1.1.0

### Creating
```bash
CATALOG=test-catalog-extra-blobs
mkdir -p ${CATALOG}/{foo,bar,baz}

opm init foo -c beta -o yaml > ${CATALOG}/foo/operator.yaml
opm init bar -c stable -o yaml > ${CATALOG}/bar/operator.yaml
opm init baz -c stable -o yaml > ${CATALOG}/baz/operator.yaml

REPO="quay.io/oc-mirror/oc-mirror-dev"
opm render ${REPO}:foo-bundle-v0.1.0 ${REPO}:foo-bundle-v0.2.0 ${REPO}:foo-bundle-v0.3.0 ${REPO}:foo-bundle-v0.3.1 --output=yaml > ${CATALOG}/foo/bundles.yaml
opm render ${REPO}:bar-bundle-v0.1.0 ${REPO}:bar-bundle-v0.2.0 ${REPO}:bar-bundle-v1.0.0 --output=yaml > ${CATALOG}/bar/bundles.yaml
opm render ${REPO}:baz-bundle-v1.0.0 ${REPO}:baz-bundle-v1.0.1 ${REPO}:baz-bundle-v1.1.0 --output=yaml > ${CATALOG}/baz/bundles.yaml
```


## test-catalog-invalid-images

Used to test OCPBUGS-33081: a catalog may contain bundles with invalid
related images (missing name, missing tag/digest, unsupported `oci://`
scheme, ...). Currently, oc-mirror fails the whole catalog collection when
any bundle has such an invalid related image, instead of skipping just the
invalid bundle. `foo.v0.9.9-invalid-related-image`'s bad related image is
never actually pulled - the string only needs to fail image reference
parsing, so it doesn't need to point at a real image.

### Contents
 * Packages: foo
 * Channels:
    - foo: beta
 * Bundles:
    - foo.v0.1.0: valid, points at the real, already-published `foo-bundle-v0.1.0` image
    - foo.v0.9.9-invalid-related-image: has a related image with no tag or digest
      (`registry.example.com/foo/operand-missing-tag`)

### Creating
```bash
CATALOG=test-catalog-invalid-images
mkdir -p ${CATALOG}/foo

opm init foo -c beta -o yaml > ${CATALOG}/foo/operator.yaml

REPO="quay.io/oc-mirror/oc-mirror-dev"
opm render ${REPO}:foo-bundle-v0.1.0 --output=yaml > ${CATALOG}/foo/bundles.yaml
# then hand-edit ${CATALOG}/foo/channels.yaml and append the invalid bundle to
# ${CATALOG}/foo/bundles.yaml - see the checked-in files for the exact content.
```


## test-catalog-tag-digest-collision

Reproduces the tag+digest cache collision (an operator's related images reference the
same repository and tag but pin different digests via the `repo:tag@sha256:...` form).
The `collisiontest` package has two channels whose bundles both reference
`oc-mirror-dev:shared-operand` but with two DIFFERENT, real digests already present in
the `oc-mirror-dev` repository. The `shared-operand` tag itself does not need to exist -
the images are pulled by digest.

### Contents
 * Packages: collisiontest
 * Channels:
    - collisiontest: alpha, stable
 * Bundles:
    - collisiontest.v1.0.0 (alpha): operand `oc-mirror-dev:shared-operand@sha256:1ce8c0...`
    - collisiontest.v2.0.0 (stable): operand `oc-mirror-dev:shared-operand@sha256:1b8392...`

### Creating
This catalog's FBC is hand-written (see the checked-in `collisiontest/` directory). The two
operand digests are existing manifests in `quay.io/oc-mirror/oc-mirror-dev`:
 * `sha256:1ce8c0187c8fe6b4be327dc848b8baf062ce1baa5096b4f5d955893d126d5b58` (foo operand)
 * `sha256:1b8392488dabcf78c82c72866d34edf6ef3d5bfb6ec00c81a377486a90d3d9ad` (foo-bundle-v0.2.0)

If those digests are ever pruned from the repository, refresh them with two distinct,
existing digests (`skopeo manifest-digest`) and update both the FBC and
`testdata/imagesetconfigs/tag_digest_collision/*.yaml`.


## Catalog building
```bash
make build # for all catalogs
make build-catalog.<catalog-name> # for specific catalog
```

## Catalog validation
```bash
opm validate <catalog>
```
