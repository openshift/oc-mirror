# Bundle

# Development!

This repo is being developed.

### Build

Not automated yet.

Requires go version 1.16.

Manual Build:
```
make
./bin/oc-bundle -h
```

### Test

Manual Unit Tests:
```bash
make test-unit
```

Manual E2E:
```bash
make test-e2e
```


Function test:
1. Download pull secret and place at ~/.docker/config.json
2. Build and test:
  ```sh
  mkdir -p test-output/src/publish
  cp data/imageset-config.yaml test-output/
  cp data/.metadata.json test-output/src/publish
  make
  ./bin/oc-bundle create full --config imageset-config.yaml --dir=test-output --log-level=debug
  ```

## Overview

Bundle is an OpenShift Client (oc) plugin that manages OpenShift installation, operator, and associated container images.

Bundle management is a two part process:

Part 1: Bundle Creation (Internet Connected)
Part 2: Bundle Publishing (Disconnected)

## Usage

See docs for [expanded overview](./docs/overview.md) and [basic usage information](./docs/usage.md)

## Bundle Spec

Note: The `imageset-config.yaml` is only used during bundle creation.

See the [config spec][config-spec] for an in-depth description of fields.

<!--
TODO: link to the following once a release is cut.
[config-spec]:https://pkg.go.dev/github.com/redhatgov/bundle/pkg/config/v1alpha1#ImageSetConfiguration
-->
[config-spec]:pkg/config/v1alpha1/config_types.go
