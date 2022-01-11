Developers: Getting Started with `oc-mirror`
====

Let's get started!

## External Dependencies
1. Make sure your go is up to date!
2. Make sure that you have `gcc` and `make` installed.
3. To run the local end-to-end test you will need `podman` or `docker`
4. jq and yq installed for JSON and YAML processing

## OSX Dependencies
Testing on OSX requires installation of several additional packages to be installed
1. Bash 5.1 or later

## Building

Please run `hack/build.sh` to build the `oc-mirror` executable for local testing
## CI

Our CI is automated using Prow. The configuration is located in the `openshift/release` project.
We are currently running automated unit tests with CI and are working on an automated end-to-end solution.

## Local Testing

We have developed local end-to-end test scripts to verify `oc-mirror` functionality with various imageset configurations.
When added a new feature or changing a current feature ensure the functionality is covered by the end-to-test located [here](../test/../../test/e2e-simple.sh)




