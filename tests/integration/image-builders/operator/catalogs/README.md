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


## Catalog building
```bash
make build # for all catalogs
make build-catalog.<catalog-name> # for specific catalog
```

## Catalog validation
```bash
opm validate <catalog>
```
