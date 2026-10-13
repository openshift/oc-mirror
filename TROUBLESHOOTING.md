# Troubleshooting
## Verbose logging
When troubleshooting, use the `--log-level=debug` flag to get debug messages. Logging information is printed to standard out, but also to a log file in the current directory called .openshift_bundle.log. Attach this file to bug reports.


## Content Selection
To troubleshoot issues with Imageset configuration generation, the `oc-mirror list releases` or `oc-mirror list operators` can be used to discover currently available content for each data type.

## Retrieve oc-mirror metadata
When submitting bug reports, include the metadata.json from the problem generated tar artifact, called an imageset. The metadata.json can be retrieved from the imageset with: `tar xf <imageset-name> /publish/.metadata.json`. Then retrieve and upload the metadata.json from  `./publish/.metadata.json`. 

## Operator Installation
To troubleshoot issues with the created file-based catalog, use the following command to get information about the problem package, ‘oc get packagemanifests -n openshift-marketplace <packagename> -o json | jq '.status.channels[]|{name: .name, currentCSV: .currentCSV}'

## Destination Registry parsing
For docker registry destinations, to preserve the same docker reference format across the ecosystem, `docker://registry` is not parsed as a registry hostname, but as an image or repository name. In order to specify a registry, qualify the hostname, or use an IP address. For example, use `docker://registry.localdomain`. `docker://localhost` works as expected, because localhost is generally treated as a special exception, not requiring a qualified domain to be parsed as a registry host.

## Error Examples
```unable to get OCI Image from oci:///$LOCATION_FOR_OCI_CATALOG: more than one image in oci, choose an image```
- It means that $LOCATION_FOR_OCI_CATALOG contains an OCI catalog with more than one manifest, and oc-mirror cannot choose which of them should be used.
- This usually happens if you copy a catalog to the same location more than once.