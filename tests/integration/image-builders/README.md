# Integration test images

Publish fixtures with `<image-name>-YYYYMMDD-<short-source-commit>` tags, for
example `quay.io/oc-mirror/oc-mirror-dev:test-catalog-20261103-2e526488d`.
Use the UTC publishing date and the Git commit containing the fixture sources.
Repository names, source-directory names, and operator versions do not change.

Published tags are permanent: never overwrite them or delete images still used
by a branch or PR. Leave legacy tags untouched, including already-versioned
bundle and release images and images referenced by digest.

## Updating a fixture

Commit the source changes first, then select a version once for the entire
build/push/sign operation:

```bash
export IMAGE_VERSION="$(date -u +%Y%m%d)-$(git rev-parse --short HEAD)"
make -C tests/integration/image-builders/operator build-catalog.test-catalog-latest
make -C tests/integration/image-builders/operator push-catalog.test-catalog-latest
make -C tests/integration/image-builders/operator sign-catalog.test-catalog-latest
```

This publishes `test-catalog-${IMAGE_VERSION}`. Other catalog tags retain their
directory name as the prefix, such as `test-catalog-extra-blobs-${IMAGE_VERSION}`.
The builders default to today's UTC date and the current source commit if
`IMAGE_VERSION` is not supplied; exporting it keeps separate commands consistent.

Update only the affected image references in
`tests/integration/testdata/imagesetconfigs/` and the corresponding Go constants
and expectations. Keep literal tags checked into Git so every PR stays pinned
to its own fixture versions. Publish the images before running CI.

The operator Makefile's default `push` and `sign` targets handle catalogs only.
When dependencies change, `push-related` builds and publishes bundles and related
images with the same version suffix; `sign-bundles` and `sign-related` sign them.
Update the bundle CSV and catalog references to use those new tags as needed,
without changing the operator's semantic version. Unchanged dependencies keep
their existing references.

For multi-architecture operator images, use the same exported `IMAGE_VERSION`:

```bash
make -C tests/integration/image-builders/operator push-multi sign-multi
```

This versions the rebuilt bundles, related images, catalog, and architecture-specific tags.
Update the generated catalog definitions and expected bundle tags together when
adopting a newly built multi-architecture catalog.

The [multilayers builder](additional_images/multilayers/README.md) uses
`multilayers-${IMAGE_VERSION}`. Existing release fixture tags remain unchanged;
publish any changed release under a new tag rather than reusing `v0.0.1`.
