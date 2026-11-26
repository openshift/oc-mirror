# Integration test images

The images of the operator catalogs and of the releases are not published: the
suite builds them when it starts, and serves them from a local origin registry.
To change one, edit its sources and run the tests:

* [`testdata/catalogs`](../testdata/catalogs/README.md), with their
  [bundles](../testdata/bundles/README.md) and
  [operands](../testdata/operands/README.md).
* [`testdata/releases`](../testdata/releases/README.md), with their
  [components](../testdata/release-components/README.md).

Only the additional images, which tests mirror as they are and which rarely
change, are built from this directory and published to Quay, such as
[multilayers](additional_images/multilayers/README.md).

Publish them with `<image-name>-YYYYMMDD-<short-source-commit>` tags, for
example `quay.io/oc-mirror/oc-mirror-dev:multilayers-20261103-2e526488d`. Use
the UTC publishing date and the Git commit containing the image sources.

Published tags are permanent: never overwrite them or delete images still used
by a branch or PR. This includes the bundle, related and release component
images published before the suite built them, which some ImageSetConfigurations
and helm charts still use as additional images.

`generate-cosign-keys.sh` generates the cosign key pair of `testdata/keys`, which
signs the published images and the images the suite builds.
