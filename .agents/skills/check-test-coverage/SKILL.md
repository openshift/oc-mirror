---
name: check-test-coverage
description: Analyze oc-mirror CLI feature coverage across integration and e2e tests, identifying untested features and gaps
user-invocable: true
---

# Check Test Coverage

Analyze which oc-mirror CLI features are tested at the integration and e2e levels, and which have gaps.

## Step-by-step process

### 1. Discover CLI features

Run these commands to enumerate all subcommands and flags:

```bash
oc-mirror --v2 --help
oc-mirror --v2 delete --help
oc-mirror --v2 list operators --help
oc-mirror --v2 list releases --help
oc-mirror --v2 version --help
```

If `oc-mirror` is not in PATH, build it first with `make build` and use `./bin/oc-mirror`.

From the output, extract:
- **Workflows**: mirrorToMirror (m2m), mirrorToDisk (m2d), diskToMirror (d2m)
- **Subcommands**: delete, list operators, list releases, version
- **Flags**: every flag from each subcommand's help output (skip hidden/deprecated flags unless they appear in tests)

### 2. Parse integration tests

Read all `*_test.go` files in `tests/integration/`. For each file, extract:
- Ginkgo `Describe`, `Context`, and `It` block labels
- Flags and workflows referenced in the test body (e.g., `--dry-run`, `--delete-id`, `--from`)
- Helper function calls that exercise specific features

Build a map of **feature -> test file + test label**.

### 3. Parse e2e tests

Read test files in `tests/e2e/test/e2e/` (primarily `oc_mirror_v2.go`). Extract:
- Test case descriptions and case IDs
- Workflows and flags exercised in each case
- Test data configs referenced from `tests/e2e/test/e2e/testdata/`

### 4. Cross-reference and classify

For each CLI feature (subcommand, flag, workflow), classify coverage:

| Level | Meaning |
|-------|---------|
| **Covered** | Tested at integration and/or e2e level with meaningful assertions |
| **Partially covered** | Only happy path, or tested at only one level when both would be appropriate |
| **Not covered** | No test exercises this feature |

Consider a feature "partially covered" if:
- It's only tested in the happy path but has error handling worth verifying
- It's a flag that modifies behavior but is only tested implicitly (e.g., the flag is set but its effect isn't asserted)

### 5. Produce the report

Present results in this format:

#### Summary table

```text
| Feature / Flag       | Integration | E2E  | Status           |
|----------------------|-------------|------|------------------|
| m2m workflow         | m2m_test.go | 73359| Covered          |
| --dry-run            | dry_run_... | -    | Partially covered|
| --secure-policy      | -           | -    | Not covered      |
| ...                  |             |      |                  |
```

#### Priority gaps

List the top uncovered or partially covered features, ordered by importance:
1. Features that affect data correctness (e.g., signature verification, archive integrity)
2. Features that affect user-facing behavior (e.g., filtering, error codes)
3. Features that are operational concerns (e.g., parallelism, profiling)

For each gap, recommend:
- Which test level is appropriate (integration vs. e2e)
- A one-line description of what the test should verify

### 6. Optional: compare with previous run

If a previous coverage report exists in the conversation history, highlight what changed: new tests added, features that moved from "not covered" to "covered", and any regressions.
