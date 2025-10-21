# Overview

## POC implementation of file based images for mirroring

### Engineering notes

#### Release

The mirror to disk flow is as follows

- cincinnati client call http ap endpoint
- endpoint return json payload
- unmarshal json
- get release image from json (could be more than one)
- download release image
- untar to release-manifests folder
- look for image-references json payload file
- parse image-reference file
- download each image to disk

#### Catalogs

The mirror to disk flow is as follows

- read imagesetconfig
- unmarshal imagesetconfig
- iterate through each catalog
- download catalog
- untar layers
- look for configs/ directory
- iterate through packages for the catalog
- look for catalog.json (cvo)
- parse catalog.json
- compare minVersion,maxVersion, channel from imageset config to unmarshaled struct
- find bundle
- get releated images
- download each releated image to disk
   
