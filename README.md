# Bundle

## Overview
Bundle is an OpenShift Client (oc) plugin that manages OpenShift installation, operator, and associated container image bundles.   

Bundle managment is a two part process:  
  
Part 1: Bundle Creation (Internet Connected)  
Part 2: Bundle Publishing (Disconnected)  

## Usage
Command: oc bundle 

Requires: bundle-config.yaml in target directory

Sub-commands: 
create
  Options:
    full
    diff
  Flags:
    --directory (string | oc bundle managed directory)
    --bundle-name (string | name of bundle archive | optional)
publish
  Flags:
    --from-bundle (string | archive name)
    --install (optional)
    --no-mirror (optional)
    --tls-verify (boolean | optional)
    --to-directory (string | oc bundle managed directory)
    --to-mirror (string | target registry url)

## Bundle Spec

Note: The `bundle-config.yaml` needs to be placed in the directory specified by the --dir flag.
Note: The `bundle-config.yaml` is only used during bundle creation.
```
bundleSpec: {{ version of bundle-config | String | Required }}
targetOCPRelease: {{ version of OCP to be bundled | String | Optional }}
operators: {{ Optional }}
  - channel: {{ update stream | string | if operators, then required}}
    full: {{ bolean }}
    operatorList: {{ list of strings | if full catalog, then required}}
additionalImages: {{ list of additional images to be bundled }}
blockedImages: {{ list of blocked images / no download }}
pullSecret: {{ cloud.redhat.com pull secret }}
bundleSize: {{ Number in GB to limit bundle file size to }}
```

