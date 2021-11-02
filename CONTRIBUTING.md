Contributing to `oc-mirror`
====

- [Contributing to `oc-mirror`](#contributing-to-oc-mirror)
  - [What should I know before I get started?](#what-should-i-know-before-i-get-started)
  - [How can I contribute?](#how-can-i-contribute)
    - [Reporting Bugs](#reporting-bugs)
    - [Requesting Enhancements](#requesting-enhancements)
    - [Your First Code Contribution](#your-first-code-contribution)
      - [Getting Started](#getting-started)
    - [Pull Requests](#pull-requests)
    - [Docs Contributions](#docs-contributions)
  - [Testing](#testing)

Welcome to our contributing guide! We are eager to receive contributions of all types. Ways to contribute include:

## What should I know before I get started?

[`oc-mirror` design](docs/design/overview)

## How can I contribute?
### Reporting Bugs
Please submit bug reports as GitHub Issues. When submitting bug reports, please include:
1. A concise title
2. Log snippets
3. The command used to execute the task.
4. The imageset-config used in the execution (if applicable)
5. Use our [template](.github/ISSUE_TEMPLATE.md)

### Requesting Enhancements
1. A concise title
2. A concise description of the modification
3. The conditions under which the modification would be relevant
3. The desired outcome of the modification
4. Provide step-by-step instructions of the enhancement
5. Explain the difference between enhancement and current functionality
6. Explain enhancement use cases
7. Explain current workaround/alternatives without the enhancement

### Your First Code Contribution

#### Getting Started
Please refer to the [developer docs](./docs/dev/getting-started.md) for information on getting started with developing on `oc-mirror`.

### Pull Requests
When submitting pull requests, please ensure the following:
1. Include unit tests if applicable
2. Update `./docs` if applicable
3. Update `./data` if modifying schema
4. Follow our [styleguides](docs/dev/styleguides.md)
5. Use our [template](.github/PULL_REQUEST_TEMPLATE.md)

### Docs Contributions

We will always need better docs! You can contribute to our `./docs` in the following ways:

1. Markdown-formatted tutorials for using `oc-mirror` in different scenarios.
2. Enhanced Developer/hacking docs
3. Linux man style docs

## Testing

To test basic functionality locally, we have developed an end to end test. Please use `make test-e2e`.

Functional testing of `oc-mirror` is difficult. Building a comprehensive test matrix is nearly impossible. If we do some thought experiments about how `oc-mirror` works, we can see the complexity quickly developing:

1. If over the lifecycle of differential use of `oc-mirror`, we have synchronized 3 openshift releases between 3 imagesets and we specify a latest openshift release for our next imageset, which prior openshift versions will have upgradeability to the incoming imageset with the latest openshift release? The answer **should** be that all previous downloads have an upgrade path to the highest incoming version because `oc-mirror` should have pulled any intermediary openshift versions needed. Is that the case? Test it out and submit an issue if it does something unexpected!







