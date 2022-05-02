Developers: Getting Started with `oc-mirror`
====

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
We are currently running automated unit and end-to-end tests with CI on all pull requets.

### Integration environment tests

When a PR has been approved to test (a step required for anyone not on the appropriate GitHub team), a smoke test in an integration environment may be requested. This environment includes an isolated VPC and a real OpenShift installation with release bits. To have Prow run this test on the PR, an approved person should comment `/test integration` on the PR to mark this optional test for inclusion.

**NOTE:** This test takes around two hours to complete. While it is designed to recover as gracefully as possible from cancellation or failure, there are cases in which it may not properly tear down an environment and leave infrastructure hanging. A team member with access to the infrastructure provider may have to manually tear down your environment. This will include [generating a new metadata.json file](https://access.redhat.com/solutions/3826921), uninstalling OpenShift, and then manually tearing down the Integration environment components with the [integration environment manual teardown script](https://github.com/jharmison-redhat/oc-mirror-e2e/blob/main/example/teardown.sh). The subdomain on which the environment is placed is required for both of those steps and can be recovered from the build log of a given PR test step run. For an example, you can see [this build log](https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/pr-logs/pull/openshift_oc-mirror/393/pull-ci-openshift-oc-mirror-main-integration/1520567128246194176/artifacts/integration/test-integration/build-log.txt) and Ctrl+F for `redhat4govaws`. Every run generates a random 6-character subdomain, and this is used as `CLUSTER_NAME` variable in the metadata.json file generation as well as the `TEARDOWN_CLUSTER` variable in the environment teardown script.

## Local Testing

We have developed local end-to-end test scripts to verify `oc-mirror` functionality with various imageset configurations.
When added a new feature or changing a current feature ensure the functionality is covered by the end-to-test located [here](../test/../../test/e2e-simple.sh)

If you would like to locally execute the smoke test in the full integration environment, you will need an AWS IAM user access key and secret and a pull secret from the [OpenShift console](https://console.redhat.com/openshift/install/aws/installer-provisioned). You can then run the following:

```sh
export AWS_ACCESS_KEY_ID=<your actual AWS access key ID>
export AWS_SECRET_ACCESS_KEY=<your actual AWS secret access key>
export CONSOLE_REDHAT_COM_PULL_SECRET='<your actual pull secret, retrieved from the link above>'
```

To run the full integration suite from the repository root, you can use the `test-integration` target, e.g.:

```sh
make test-integration
```

If your integration test fails and leaves the environment hanging half-open, you can tear it down with the following:

```sh
cd test/integration
make delete
```

To run a specific phase of the integration test (for example, to provision and run the test matrix in the environment, without deleting it automatically), you can run the following from the `test/integration` folder directly:

```sh
make create test
```

Additional debugging of the disconnected integration environment (including hopping through a connected host to reach the isolated bastion) can be performed with the `test/integration/connect.sh` script, which accepts a single argument - the name of the host you want to connect to. It can be one of `registry`, `proxy`, or `bastion`.

For custom test scenarios, or cases where your test may break the `oc-mirror` command line arguments or configuration API schema, the `vars.yml` file contains some variables to help you. Further documentation on the integration environment is available at the [ansible collection repository](https://github.com/jharmison-redhat/oc-mirror-e2e) for it.
