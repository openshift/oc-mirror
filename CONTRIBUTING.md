# Contributing to `oc-mirror`

- [Contributing to `oc-mirror`](#contributing-to-oc-mirror)
  - [Contribution Expectations](#contribution-expectations)
  - [Development](#development)
    - [Common Commands](#common-commands)
  - [Testing](#testing)
    - [CLI Integration Tests](#cli-integration-tests)
  - [Pull Requests](#pull-requests)
  - [Reporting Bugs](#reporting-bugs)
  - [Requesting Enhancements](#requesting-enhancements)
  - [Docs Contributions](#docs-contributions)

Welcome to our contributing guide! We are eager to receive contributions of all types. Here are some ways to contribute:

## Contribution Expectations

1. Follow the repository's existing patterns and quality standards. Submit changes that work as expected and include
   appropriate tests and documentation.
2. You are responsible for every submitted change, including agent-generated code. Understand its purpose,
   implementation, and impact well enough to explain them.
3. Keep contributions focused, preferably with a single, clear purpose. Discuss large-scale or non-trivial changes
   with the maintainers before starting work. Large-scale or non-trivial changes that are not previously discussed with the maintainers might get closed. 
4. Make sure that your contributions have a clear purpose or impact. In the case of bug fixes, make sure they address a real problem that can be reproduced and potentially affect users. 
   Contributions without a clear purposei or bug fixes without real user impact will be closed.

## Development

Active development happens in the root directory (v2). Code under `v1/` is deprecated. Please do not submit changes there.

### Common Commands

Run these commands from the repository root unless noted otherwise:

| Command | Purpose |
|---------|---------|
| `make build` | Build `bin/oc-mirror` with the required build tags and embedded v1 executable. Use this instead of `go build` directly. |
| `make test-unit` | Run short tests under `internal/pkg/` with race detection and coverage. |
| `make test-integration` | Run selected in-process integration tests under `internal/pkg/`. |
| `make test-integration-cli` | Build the binary and run the black-box CLI integration suite under `tests/integration/`. |
| `make verify` | Run golangci-lint. |
| `make tidy` | Run `go mod tidy`, which can update dependency files. |
| `make format` | Check Go formatting; this does not reformat files. |
| `make vet` | Run `go vet` with the required build tags. |
| `make sanity` | Run tidy, formatting, and vet checks, then fail if there are unstaged tracked changes. |
| `make clean` | Remove build artifacts. |

Before submitting Go changes, run the relevant tests and the following checks. Review any dependency changes made
by `make tidy` and include only those required by your contribution.

```bash
make tidy
make format
make vet
make verify
```

## Testing

See our [testing strategy](docs/testing/README.md) for guidance on how testing is done in `oc-mirror`.

### CLI Integration Tests

The Ginkgo suite under `tests/integration/` is a separate Go module. It tests the built CLI rather than importing
the root module's internal packages. Run the full suite from the repository root with:

```bash
make test-integration-cli
```

To rerun tests without rebuilding each time, or to run a focused test, build once and enter the integration module:

```bash
make build
cd tests/integration

# Run the full suite.
go test -v -count=1 -timeout 30m ./...

# Run tests whose Ginkgo descriptions match a regular expression.
go test -v -count=1 -timeout 30m . -ginkgo.focus '<test description or regex>'
```

Rebuild after changing the CLI code. The runner defaults to `../../bin/oc-mirror`; set `OC_MIRROR_BINARY` to an
absolute path to test another binary.

## Pull Requests

1. Make sure all commits are signed, otherwise GitHub will refuse to merge
2. Use descriptive commit messages and keep each commit self-contained and focused.
3. Prefer a single commit or a small number of commits per PR. Squash when necessary.
4. Use our [pull request template](.github/PULL_REQUEST_TEMPLATE.md), including a description of the change and how
   it was tested.
5. Address review comments from human maintainers and automated reviewers. Automated suggestions still require critical review; do not apply them blindly.

> [!IMPORTANT]
> Pull requests with failing pipelines or unaddressed review comments **will be closed after two weeks of inactivity**,
> unless marked as Draft or Work In Progress.

## Reporting Bugs

Please submit bug reports as GitHub Issues using our [template](.github/ISSUE_TEMPLATE.md). Include:

1. A concise title
2. Log snippets
3. The command used to execute the task
4. The imageset-config used in the execution (if applicable)

## Requesting Enhancements

Please submit enhancement requests as GitHub Issues. Note that prioritization happens in the RFE project in Jira.

1. A concise title and description of the modification
2. The conditions under which the modification would be relevant
3. The desired outcome and how it differs from current functionality
4. Use cases for the enhancement
5. Current workaround/alternatives without the enhancement

## Docs Contributions

We welcome improvements to our `./docs`:

1. Markdown-formatted tutorials for using `oc-mirror` in different scenarios
2. Enhanced developer/hacking docs
