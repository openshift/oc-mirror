Contributing to `bundle`
====

Welcome to our contributing guide! We are eager to receive contributions of all types. Ways to contribute include:

- [Submitting issues for bugs/enhancements/feature requests](#Submitting-Issues)
- [Contributing code through Pull Requests](#Code-Contributions)
- [Contributing docs through pull requests](#Docs-Contributions)
- [Testing](#Testing)  

## Submitting Issues

### Bugs
Please submit bug reports as GitHub Issues. When submitting bug reports, please include:
1. A concise title
2. A detailed description of the problem
3. Environmental factors that may have influenced the outcome.
4. Log snippits
5. The command used to execute the task.
6. The imageset-config used in the execution (if applicable)

### Enhancements/Feature Requests
1. A concise title
2. A concise description of the modification
3. The conditions under which the modification would be relevant
3. The desired outcome of the modification

## Code Contributions

### Getting Started
Please refer to the [developer docs](./docs/dev/getting-started.md) for information on getting started with developing on `bundle`.

### Pull Requests
When submitting pull requests, please ensure the following:
1. Code formatted with `go fmt`
2. Include unit tests if applicable
3. Update `./docs` if applicable
4. Update `./data` if modifying schema
5. Squash commits
6. Concise PR title
7. Provide a detailed description of the PR and why it is needed.

## Docs Contributions

We will always need better docs! You can contribute to our `./docs` in the following ways:

1. Markdown formatted tutorials for using `bundle` in different scenarios.
2. Enhanced Developer/hacking docs
3. Linux man style docs

## Testing

Functional testing of `bundle` is difficult. Building a comprehensive test matrix is nearly impossible. If we do some thought experiments about how `bundle` works, we can see the complexity quickly developing:

1. If over the lifecycle of differential use of `bundle`, we have synchronized 3 openshift releases between 3 imagesets and we specify a latest openshift release for our next imageset, which prior openshift versions will have upgradeability to the incoming imageset with the latest openshift release? The answer **should** be that all previous downloads have an upgrade path to the highest incoming version, because `bundle` should have pulled any intermediary openshift versions needed. Is that the case? Test it out and submit an issue if does something unexpected!








