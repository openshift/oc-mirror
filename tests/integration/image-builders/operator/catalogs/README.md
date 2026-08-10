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


## Catalog building
```bash
make build # for all catalogs
make build-catalog.<catalog-name> # for specific catalog
```

## Catalog validation
```bash
opm validate <catalog>
```
