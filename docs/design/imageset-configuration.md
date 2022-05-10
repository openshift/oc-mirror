Design: oc-mirror Imageset Configuration
===

- [Design: oc-mirror Imageset Configuration](#design-oc-mirror-imageset-configuration)
- [Content Types](#content-types)
  - [Platforms](#platforms)
  - [Operators](#operators)
  - [Additional Images](#additional-images)
  - [Helm Chart](#helm-chart)
- [Limitations](#limitations)
  - [Platforms and Catalogs](#platforms-and-catalogs)
  - [Helm Charts](#helm-charts)


# Content Types

## Platforms

Release channels for container management platforms can be specified for mirroring. 
Currently, OpenShift Container Platform is supported by all `oc-mirror` commands.
OKD is a supported type in the imageset configuration, but not by the `oc-mirror` list commands.

## Operators

Catalog images can be specified for mirroring. The associated bundles will be processed
to determine all bundle images and dependency images needed.
A file-based catalog is stored in the imageset and used to build
a custom catalog image in the target registry.

## Additional Images

Individual images can be specified in an imageset configuration. If no tag is specified, the "latest" tag will be assigned.

## Helm Chart

Helm chart locations can be specified for download and `oc-mirror` will detect and mirror images contained in the chart
on a best-effort basis. Local and remote Helm charts can be used. `oc-mirror` will search well-known locations, but custom image locations can be passed with the `imagePaths` key. Example:


```
apiVersion: mirror.openshift.io/v1alpha2
kind: ImageSetConfiguration
mirror:
  helm:
    repositories:
      - name: podinfo
        url: https://stefanprodan.github.io/podinfo
        charts:
          - name: podinfo
            version: 5.0.0
            imagePaths: 
            - "{.spec.template.spec.custom[*].image}"
```

# Limitations

## Platforms and Catalogs

- Duplicate release channel names: A release channel can only be configured one time under the `platform` key. `oc-mirror` will return an error when this is violated.
- Duplicate target catalog names - A target catalog name is generated from the source catalog name and any target information set with the keys `targetName` and `targetTag`. The target names must be unique. `oc-mirror` will return an error when this is violated.

OKD is supported by the mirroring process but not the discovery `list` tools.

## Helm Charts

- Private repositories are currently not supported. 
- Helm charts that require alterations to the `values.yaml` to render are not currently supported.




