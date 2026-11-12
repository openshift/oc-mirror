# Contributing to `oc-mirror`

- [Contributing to `oc-mirror`](#contributing-to-oc-mirror)
  - [General Coding Guidelines](#general-coding-guidelines)
  - [Contribution Expectations](#contribution-expectations)
  - [Development](#development)
    - [Common Commands](#common-commands)
  - [Testing](#testing)
    - [CLI Integration Tests](#cli-integration-tests)
  - [Pull Requests](#pull-requests)
  - [Reporting Bugs and RFEs](#reporting-bugs-and-rfes)
  - [Docs Contributions](#docs-contributions)

Welcome to our contributing guide! We are eager to receive contributions of all types. Here are some ways to contribute:

## General Coding Guidelines

1. Write simple and understandable code that is properly tested and documented.
2. Don't reinvent the wheel. Prefer using well-established libraries when possible over building functionality from scratch when it already exists somewhere else.
Some of our core dependencies are:
   - [container-libs](https://github.com/podman-container-tools/container-libs) for container image copying, storage,
   and shared container utilities.
   - [go-containerregistry](https://github.com/google/go-containerregistry) for container image inspection and manipulation.
   - [Distribution](https://github.com/distribution/distribution) for the embedded container registry.
   - [operator-registry](https://github.com/operator-framework/operator-registry) and
   - [Helm](https://github.com/helm/helm) for chart loading and rendering.
   - [Cobra](https://github.com/spf13/cobra) for CLI commands and flags.
   - [OpenShift API](https://github.com/openshift/api) and
    [Kubernetes API machinery](https://github.com/kubernetes/apimachinery) for cluster resource types and validation.
3. Respect the scope of oc-mirror. `oc-mirror` is a tool that focuses on efficient and convenient mirroring of container
images and their related artifacts. If you need something else consider if other tools could be used
for that particular task before trying to incorporate it into `oc-mirror`.
4. When implementing a new feature or bug fix, consider if it could be done in one of our upstream dependencies (e.g. container-libs, OLM...). Prefer implementing shared functionality
there instead of duplicating it in oc-mirror.
5. Consider the impact, reach, and recoverability of failures, and scale review and verification accordingly.
 Changes to image copying (`mirror`), archives/state (`archive`, `history`), deletion (`delete`), trust (`signature`),
 and catalog rebuilding (`imagebuilder`, `operator`, `manifest`) deserve particular care.
 Presentation-only changes (`emoji`, `spinners`, `log`, version-command display), isolated tests, and dry-run are generally safer. 

## Contribution Expectations

1. Follow the repository's existing patterns and quality standards. Submit changes that work as expected and include
appropriate tests and documentation.
2. You are responsible for every submitted change, including agent-generated code. Understand its purpose,
implementation, and impact well enough to explain them.
3. Keep contributions focused, preferably with a single, clear purpose. Discuss large-scale, non-trivial or breaking changes
with the maintainers before starting work. Large-scale or non-trivial changes that are not previously discussed with the maintainers might get closed.
4. Make sure that your contributions have a clear purpose or impact. In the case of bug fixes, make sure they address a real problem that can be reproduced and potentially affect users. Contributions without a clear purpose or bug fixes without real user impact will be closed.

## Development

Active development happens in the root directory (v2). Code under `v1/` is deprecated. Please do not submit changes there.

### Common Commands

Run these commands from the repository root unless noted otherwise:


| Command                     | Purpose                                                                                                                 |
| --------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `make build`                | Build `bin/oc-mirror` with the required build tags and embedded v1 executable. Use this instead of `go build` directly. |
| `make test-unit`            | Run short tests under `internal/pkg/` with race detection and coverage.                                                 |
| `make test-integration`     | Run selected in-process integration tests under `internal/pkg/`.                                                        |
| `make test-integration-cli` | Build the binary and run the black-box CLI integration suite under `tests/integration/`.                                |
| `make verify`               | Run golangci-lint.                                                                                                      |
| `make tidy`                 | Run `go mod tidy`, which can update dependency files.                                                                   |
| `make format`               | Check Go formatting; this does not reformat files.                                                                      |
| `make vet`                  | Run `go vet` with the required build tags.                                                                              |
| `make sanity`               | Run tidy, formatting, and vet checks, then fail if there are unstaged tracked changes.                                  |
| `make clean`                | Remove build artifacts.                                                                                                 |


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

## Reporting Bugs and RFEs

Jira is our preferred way of reporting bugs and requests for enhancements (RFEs).

## Docs Contributions

We welcome improvements to our `./docs`:

1. Markdown-formatted tutorials for using `oc-mirror` in different scenarios
2. Enhanced developer/hacking docs
