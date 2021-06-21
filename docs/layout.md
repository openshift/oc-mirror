# Bundle Layout

## /cmd/oc-bundle:

create.go: create subcommands
log.go: logging resources
main.go: Root command
publish.go: publish subcommands

## /pkg/bundle

individual.go: individual image handling
bundle.go: metadata handling
create.go: create top level functions
doc.go: godoc
init.go: directory and file management
operator.go: operator handling
publish.go: publish top level functions
release.go: OCP release handling
spec.go: package types 

