Contributing to `oc-mirror`
====

- [Contributing to `oc-mirror`](#contributing-to-oc-mirror)
  - [Development](#development)
  - [Reporting Bugs](#reporting-bugs)
  - [Requesting Enhancements](#requesting-enhancements)
  - [Pull Requests](#pull-requests)
  - [Testing](#testing)
  - [Docs Contributions](#docs-contributions)

Welcome to our contributing guide! We are eager to receive contributions of all types. Here are some ways to contribute:

Development
-----------

Active development happens in the root directory (v2). Code under `v1/` is deprecated. Please do not submit changes there.

Development workflow:

```bash
make build            # compile the oc-mirror binary (do not use `go build` directly)
make test-unit        # run unit tests
make test-integration # run integration tests
make verify           # run golangci-lint
make sanity           # run tidy, format, and vet checks
make clean            # clean build artifacts
```

Always run `make sanity` before committing.

Reporting Bugs
--------------

Please submit bug reports as GitHub Issues using our [template](.github/ISSUE_TEMPLATE.md). Include:
1. A concise title
2. Log snippets
3. The command used to execute the task
4. The imageset-config used in the execution (if applicable)

Requesting Enhancements
-----------------------

Please submit enhancement requests as GitHub Issues. Note that prioritization happens in the RFE project in Jira.

1. A concise title and description of the modification
2. The conditions under which the modification would be relevant
3. The desired outcome and how it differs from current functionality
4. Use cases for the enhancement
5. Current workaround/alternatives without the enhancement

Pull Requests
-------------

When submitting pull requests, please ensure the following:
1. Make sure all commits are signed, otherwise github will refuse to merge
2. Include unit tests if applicable
3. Update `./docs` if applicable
4. Use our [template](.github/PULL_REQUEST_TEMPLATE.md)

Testing
-------

See our [testing strategy](docs/testing/README.md) for guidelines on testing levels and principles.

Docs Contributions
------------------

We welcome improvements to our `./docs`:
1. Markdown-formatted tutorials for using `oc-mirror` in different scenarios
2. Enhanced developer/hacking docs
