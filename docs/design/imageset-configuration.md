# oc-mirror Imageset Configuration
===

- [# oc-mirror Imageset Configuration](#-oc-mirror-imageset-configuration)
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

Container management platforms release channels can be specified for mirroring. 
Currently, OpenShift Container Platform is supported by all `oc-mirror` commands.
OKD is a supported type in the imageset configuration, but not by the `oc-mirror` list commands.

## Operators

Catalog images can be specified for mirroring. Bundle images and all the dependencies
will be mirrored and the catalog with be rebuilt by `oc-mirror` with the custom file-based catalog
generated during package filtering.


## Additional Images

Single images to mirrored can be specified in an image set. If no tag is specified, the latest tag will be assigned.

## Helm Chart

Helm chart locations can be specified for download and `oc-mirror` will detect and mirror images contained in the chart
on a best-effort bases. Local Helm charts as well as public Helm charts in a repository can be used. `oc-mirror` will search well-known locations, but custom image locations can be passed with the `imagepath` key. Example:


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
Current limitations of the imageset configuration is duplication when it comes to specifying channels and catalogs. A catalog or release
channel can only be specified and configured one time. `oc-mirror` will return an error is this is violated.

OKD is supported by the mirroring processes, but the discovery `list` tools do not support OKD.

## Helm Charts

- Private repositories are current not supported. 
- Helm charts that require alterations to the values.yaml to render are no currently supported.




