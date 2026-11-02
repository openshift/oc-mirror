# Catalogs

Mock file-based catalogs (FBC) used in tests. Each directory is built into the image
`localhost:5001/oc-mirror/oc-mirror-dev:<directory name>` at the start of the test
session (see [../../README.md](../../README.md)).

## How catalogs are built

Each package directory holds an opm [basic catalog template](https://olm.operatorframework.io/docs/reference/catalog-templates/#basic-template)
(`basic-template.yaml`): the package, its channels, and the bundle images. At test
time, it is rendered with opm's library against the bundle images in the source
registry, exactly like:

```bash
opm alpha render-template basic <package>/basic-template.yaml
```

Any other YAML file is added to the catalog as is (hand-written FBC, e.g. extra blobs
or bundles that can't be rendered from an image). These files, and the bundle
manifests, can reference image digests with `{{ digest "<repository>:<tag>" }}`.

To create a template from an existing FBC:

```bash
opm alpha convert-template basic <fbc-file> -o yaml > basic-template.yaml
```

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

## test-catalog-multi

Same as `test-catalog-latest`, but built as a linux/amd64 + linux/arm64 image index,
and the channel heads `foo.v0.3.1` and `bar.v1.0.0` reference the multi-arch bundles
`foo-bundle-v0.3.1-multi` and `bar-bundle-v1.0.0-multi`.

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

## test-catalog-prune

### Contents
 * Packages: foo, bar
 * Channels:
    - foo: beta
    - bar: alpha
 * Bundles:
    - foo: v0.1.0, v0.1.1
    - bar: v0.1.0

## test-catalog-prune-diff

### Contents
 * Packages: foo, bar
 * Channels:
    - foo: beta
    - bar: alpha, stable
 * Bundles:
    - foo: v0.2.0
    - bar: v0.1.0

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

## test-catalog-invalid-images

Used to test OCPBUGS-33081: a catalog may contain bundles with invalid
related images (missing name, missing tag/digest, unsupported `oci://`
scheme, ...). Currently, oc-mirror fails the whole catalog collection when
any bundle has such an invalid related image, instead of skipping just the
invalid bundle. `foo.v0.9.9-invalid-related-image` is hand-written in `foo/invalid-bundle.yaml`;
its bad related image is never actually pulled - the string only needs to fail image reference
parsing, so it doesn't need to point at a real image.

### Contents
 * Packages: foo
 * Channels:
    - foo: beta
 * Bundles:
    - foo.v0.1.0: valid, points at the `foo-bundle-v0.1.0` image
    - foo.v0.9.9-invalid-related-image: has a related image with no tag or digest
      (`registry.example.com/foo/operand-missing-tag`)

## test-catalog-tag-digest-collision

Reproduces the tag+digest cache collision (an operator's related images reference the
same repository and tag but pin different digests via the `repo:tag@sha256:...` form).
The `collisiontest` package has two channels whose bundles both reference
`oc-mirror-dev:shared-operand`, but pinned to the digests of two different images:
`operand` and `operand-alt`. The `shared-operand` tag itself does not exist - the
images are pulled by digest.

This catalog is hand-written FBC (no template).

### Contents
 * Packages: collisiontest
 * Channels:
    - collisiontest: alpha, stable
 * Bundles:
    - collisiontest.v1.0.0 (alpha): operand `oc-mirror-dev:shared-operand@<digest of operand>`
    - collisiontest.v2.0.0 (stable): operand `oc-mirror-dev:shared-operand@<digest of operand-alt>`
