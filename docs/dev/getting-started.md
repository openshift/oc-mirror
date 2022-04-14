Developers: Getting Started with `oc-mirror`
====
- [Developers: Getting Started with `oc-mirror`](#developers-getting-started-with-oc-mirror)
  - [External Dependencies](#external-dependencies)
  - [Building](#building)
  - [CI](#ci)
  - [Local Testing](#local-testing)

Let's get started!

## External Dependencies
1. Make sure your go is up to date!
2. Make sure that you have `gcc` and `make` installed.
3. To run the local end-to-end test you will need `podman` or `docker`
4. To run the full end-to-end test in the integration environment you will need `python3`, `curl`, and `unzip`

## Building

Please run `hack/build.sh` to build the `oc-mirror` executable for local testing. If you do not have uncommitted changes that you are testing, you should run `hack/build.sh --clean` in order to clean the working directory of any artifacts possibly left over from your work or a previous build/test.

## CI

Our CI is automated using Prow. The configuration is located in the `openshift/release` project.
We are currently running automated unit tests with CI and are working on an automated end-to-end solution.

## Local Testing

We have developed local end-to-end test scripts to verify `oc-mirror` functionality with various imageset configurations.
When added a new feature or changing a current feature ensure the functionality is covered by the end-to-test located [here](../test/../../test/e2e-simple.sh)

If you would like to conduct a more exhaustive end-to-end test in a full integration environment, including an isolated VPC and a real OpenShift installation, you will need an AWS IAM user access key and secret and a pull secret from the [OpenShift console](https://console.redhat.com/openshift/install/aws/installer-provisioned). You can then run the following:

```sh
export AWS_ACCESS_KEY_ID=<your actual AWS access key ID>
export AWS_SECRET_ACCESS_KEY=<your actual AWS secret access key>
export CONSOLE_REDHAT_COM_PULL_SECRET='<your actual pull secret, retrieved from the link above>'
```

To run the full E2E integration suite from the repository root, you can use the `test-integration` target, e.g.:

```sh
make test-integration
```

If your integration test fails and leaves the environment hanging half-open, you can tear it down with the following:

```sh
cd test/integration
make delete
```

To run a specific phase of the E2E integration (for example, to provision and run the test matrix in the environment, without deleting it automatically), you can run the following from the `test/integration` folder directly:

```sh
make create test
```

Additional debugging of the disconnected integration environment (including hopping through a connected host to reach the isolated bastion) can be performed with the `test/integration/connect.sh` script, which accepts a single argument - the name of the host you want to connect to. It can be one of `registry`, `proxy`, or `bastion`.

For custom test scenarios, or cases where your test may break the `oc-mirror` command line arguments or configuration API schema, the `vars.yml` file contains some variables to help you. Further documentation on the E2E integration environment is available at the [ansible collection repository](https://github.com/jharmison-redhat/oc-mirror-e2e) for it.
