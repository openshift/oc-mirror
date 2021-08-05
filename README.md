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

Maunal Unit Tests:
```bash
make test-unit
```

Manual E2E:
```bash
make test-e2e
```


Function test:
1. Download pull secret and place at ~/.docker/config.josn
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

Bundle managment is a two part process:

Part 1: Bundle Creation (Internet Connected)
Part 2: Bundle Publishing (Disconnected)

## Usage

```
Command: oc bundle

Sub-commands:
create
  Options:
    full
    diff
  Flags:
    --directory (string | oc bundle managed directory)
    --bundle-name (string | name of bundle archive | optional)
    --config (string | path to imageset-config.yaml | defaults to current directory)
publish
  Flags:
    --from-bundle (string | archive name)
    --install (optional)
    --no-mirror (optional)
    --tls-verify (boolean | optional)
    --to-directory (string | oc bundle managed directory)
    --to-mirror (string | target registry url)
```

## Bundle Spec

Note: The `imageset-config.yaml` is only used during bundle creation.

See the [config spec][config-spec] for an in-depth description of fields.

<!--
TODO(estroz): link to the following once public
[config-spec]:https://pkg.go.dev/github.com/redhatgov/bundle/pkg/config/v1alpha1#ImageSetConfiguration
-->
[config-spec]:pkg/config/v1alpha1/config_types.go
