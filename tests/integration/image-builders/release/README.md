# Overview

This is a hand crafted release payload used in oc-mirror-e2e testing of release images

## Creating a release image

### Build and push the base image

```bash

./create-release.sh

```

Once the create-release bash script has executed succesfully

Execute the following command

```bash

skopeo copy oci:release-payload docker://quay.io/oc-mirror/release/test-release-index:v0.0.1 

```



