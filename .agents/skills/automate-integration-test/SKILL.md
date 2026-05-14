---
name: automate-integration-test
description: Convert a manual test case description into a Ginkgo oc-v2 integration test for oc-mirror
user-invocable: true
---

# Automate Manual Test Case to Go

Convert a manual test case description into a Ginkgo v2 integration test for the oc-mirror integration test suite.

## Test Images

These are minimal images we generated and host ourselves to keep tests fast and self-contained, avoiding dependencies on external images that might change frequently.

- Catalog: `quay.io/oc-mirror/oc-mirror-dev:test-catalog-latest`
- Release: `quay.io/oc-mirror/release/test-release-index:v0.0.1`
- Additional: `quay.io/openshifttest/hello-openshift@sha256:61b8f5...`

## Step-by-step process

### 1. Read project conventions

Read `AGENTS.md` at the project root for architecture, conventions, and pitfalls.

### 2. Clarify if needed

If the test case is ambiguous, ask before writing code:
- Which mirror mode? (`mirrorToMirror`, `mirrorToDisk` + `diskToMirror`, or both)
- Does it include a delete workflow (phase 1 + phase 2)?
- Is a new ISC or DISC YAML needed, or does an existing one suffice?
- Should the test go in an existing file or a new one?

### 3. Check for duplicate or superfluous coverage

Read existing test files to ensure the scenario isn't already covered. If it is, tell the user and suggest what would add value instead.
Also consider if one of the existing tests could be extended to cover the scenario, without blurrying the testing boundaries.

### 4. Discover available APIs

Read these files to learn the current patterns - do not guess or reinvent:
- `tests/integration/integration_suite_test.go` - globals and lifecycle
- `tests/integration/helpers_test.go` - assertion helpers
- An existing test file matching the scenario type - to see calling conventions

Prefer existing helpers. If a new one is needed, follow the `expect*` naming pattern.

### 5. Analyze oc-mirror APIs if needed

If technical details are unclear:
- Check for a local copy at `./oc-mirror`
- Otherwise ask the user for a path or permission to clone from `github.com:openshift/oc-mirror.git`

### 6. Write the test

Follow the patterns from existing tests. Key rules:
- Match the structure of existing tests in the target file - `Describe`, `BeforeEach`/`AfterEach`, `It`, `By` steps
- Consider if the scenario covers all critical paths, and suggest new ones
- Implement meaningful assertions - avoid surface-level or redundant checks
- For error scenarios, assert on `result.ExitCode` and `result.Stderr` content

### 7. Choose or create ISC/DISC configs

Check existing configs in `tests/integration/testdata/imagesetconfigs/` first. Only create new ones if needed, using the same YAML structure and the test images listed in `AGENTS.md`.

### 8. Review the code

Before running, review the generated code for:
- **Correctness**: assertions actually verify the scenario, no false positives or dead steps
- **Efficiency**: no redundant operations, unnecessary variables, or repeated lookups
- **Cleanliness**: no superfluous comments, unused variables, or boilerplate that adds nothing - every line should earn its place

### 9. Run the tests

Ask permission, then run. The `registry` binary must be in PATH - ask the user for its location if unknown:
```bash
PATH=$PATH:<path-to-registry-bin> go test -v ./tests/integration/ --ginkgo.focus "<test label>"
```

### 10. Output

Provide the complete Go test code, any new YAML configs, and a one-line summary of where each file goes. Be concise - the user knows the project.
