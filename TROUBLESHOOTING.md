# Troubleshooting
## Verbose logging
When troubleshooting, use the `--log-level=debug` flag to get debug messages. Logging information is printed to standard out, but also to a log file in the current directory called .openshift_bundle.log. Attach this file to bug reports.


## Content Selection
To troubleshoot issues with Imageset configuration generation, the `oc-mirror list releases` or `oc-mirror list operators` can be used to discover currently available content for each data type.

## Retrieve oc-mirror metadata
When submitting bug reports, include the metadata.json from the problem generated tar artifact, called an imageset. The metadata.json can be retrieved from the imageset with: `tar xf <imageset-name> /publish/.metadata.json`. Then retrieve and upload the metadata.json from  `./publish/.metadata.json`. 

## Operator Installation
To troubleshoot issues with the created file-based catalog, use the following command to get information about the problem package, ‘oc get packagemanifests -n openshift-marketplace <packagename> -o json | jq '.status.channels[]|{name: .name, currentCSV: .currentCSV}'