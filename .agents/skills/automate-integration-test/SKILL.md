---
name: automate-integration-test
description: Convert a manual test case description into a Ginkgo oc-v2 integration test for oc-mirror
user-invocable: true
---

# Automate Manual Test Case to Go

Convert a manual test case description into a Ginkgo v2 integration test for the oc-mirror integration test suite.

## Test Images

The suite uses minimal fixture images to keep tests fast and predictable. Reuse the complete, pinned image references
in `tests/integration/testdata/imagesetconfigs/` and the matching test constants. Do not use abbreviated digests or
substitute mutable `latest` tags.

See `tests/integration/image-builders/README.md` before publishing a new fixture version.

## Step-by-step process

### 1. Read project conventions

Read `AGENTS.md` for agent workflows, `CONTRIBUTING.md` for contributor requirements and test commands,
`ARCHITECTURE.md` for the system design, and `docs/testing/README.md` for the testing strategy.

### 2. Clarify if needed

If the test case is ambiguous, ask before writing code:
- Which mirror mode? (`mirrorToMirror`, `mirrorToDisk` + `diskToMirror`, or both)
- Does it include a delete workflow (phase 1 + phase 2)?
- Is a new ISC or DISC YAML needed, or does an existing one suffice?
- Should the test go in an existing file or a new one?

### 3. Check for duplicate or superfluous coverage

Read existing test files to ensure the scenario isn't already covered. If it is, tell the user and suggest what would add value instead.
Also consider if one of the existing tests could be extended to cover the scenario, without blurring the testing boundaries.

### 4. Discover available APIs

Read these files to learn the current patterns - do not guess or reinvent:
- `tests/integration/integration_suite_test.go` - globals and lifecycle
- `tests/integration/helpers_test.go` - assertion helpers
- An existing test file matching the scenario type - to see calling conventions

Prefer existing helpers. If a new one is needed, follow the `expect*` naming pattern.

### 5. Analyze oc-mirror APIs if needed

Inspect the API types in `internal/pkg/api/v2alpha1/` and the relevant implementation under `internal/pkg/`
in this checkout. Do not look for a nested `./oc-mirror` checkout or clone another copy of this repository.

### 6. Write the test

Follow the patterns from existing tests. Key rules:
- Match the structure of existing tests in the target file - `Describe`, `BeforeEach`/`AfterEach`, `It`, `By` steps
- Consider if the scenario covers all critical paths, and suggest new ones
- Implement meaningful assertions - avoid surface-level or redundant checks
- For error scenarios, assert on `result.ExitCode` and `result.Stderr` content

### 7. Choose or create ISC/DISC configs

Check existing configs in `tests/integration/testdata/imagesetconfigs/` first. Only create new ones if needed,
using the same YAML structure and the pinned fixture references described in the Test Images section above.

### 8. Review the code

Before running, review the generated code for:
- **Correctness**: assertions actually verify the scenario, no false positives or dead steps
- **Efficiency**: no redundant operations, unnecessary variables, or repeated lookups
- **Cleanliness**: no superfluous comments, unused variables, or boilerplate that adds nothing - every line should earn its place

### 9. Run the tests

Ask for permission before running tests. Follow the CLI integration test instructions in `CONTRIBUTING.md`.
The suite is a separate Go module, so run focused tests from `tests/integration/`, not the repository root:

```bash
make build
cd tests/integration
go test -v -count=1 -timeout 30m . -ginkgo.focus '<test description or regex>'
```

These commands start at the repository root. The suite starts an in-process registry; no registry executable or
`PATH` override is needed. Use `make test-integration-cli` from the repository root to build and run the full suite.

### 10. Output

Provide the complete Go test code, any new YAML configs, and a one-line summary of where each file goes. Be concise - the user knows the project.
