# Bundle Layout

## cmd/oc-bundle:

* main.go: Root command
* create.go: create subcommands
* log.go: logging resources
* publish.go: publish subcommands

## pkg/create

* create.go: create top level functions

## pkg/publish

* publish.go: publish top level functions

## pkg/bundle

* cincinnati.go: Temporary import due to needing to change API request. Needs to PR CVO.
* init.go: directory and file management
* operator.go: operator handling
* release.go: OCP release handling

## pkg/config

Contains imageset and metadata [configuration][component-config] specifications, and decoders/encoders.

* load.go: load a particular specification version.
* metadata.go: metadata encoding.
* v1alpha1
  * config_types.go: ImageSetConfiguration specification and decoder.
  * metadata_types.go: Metadata specification and decoder.
  * register.go: component config type metadata.


[component-config]:https://docs.google.com/document/d/1FdaEJUEh091qf5B98HM6_8MS764iXrxxigNIdwHYW9c/edit#heading=h.3l8kh6i1ko2h
