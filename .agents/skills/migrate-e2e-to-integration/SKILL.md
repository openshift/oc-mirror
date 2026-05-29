---
name: migrate-e2e-to-integration
description: Migrate an oc-mirror e2e test case to the integration test suite, translating framework, registry, invocation, and assertion patterns
user-invocable: true
---

# Migrate E2E Test Case to Integration

Convert an existing e2e test case from `tests/e2e/test/e2e/oc_mirror_v2.go` into an integration test under `tests/integration/`.

## Input

The user provides one of:
- An e2e case number (e.g., 72973)
- A test description or keyword

## Step-by-step process

### 1. Read project conventions

Read `AGENTS.md` at the project root, and the documents at `docs/testing` for architecture, conventions, and pitfalls.

### 2. Locate and analyze the e2e test case

Search `tests/e2e/test/e2e/oc_mirror_v2.go` for the case number or description. Extract:
- What scenario it tests (workflow, flags, edge case)
- How oc-mirror is invoked (subcommand, flags, config)
- What it asserts (images mirrored, files created, errors expected)
- What test data config it uses (from `tests/e2e/test/e2e/testdata/`)

Do not attempt a 1:1 translation of the test case, consider if it makes sense at a high level, if it's already covered at the integration or unit levels, if the assertions are sound, if it could be made more efficient, etc.

### 3. Check for existing integration coverage

Read integration test files in `tests/integration/` to verify the scenario isn't already covered. If it is, tell the user and suggest what additional value the e2e case provides, if any.

### 4. Present the migration plan

Before writing any code, present a detailed plan to the user and wait for approval. The plan must include:

- **E2E summary**: What the original test does — scenario, flags, config, assertions.
- **Translation approach**: How each part maps to integration patterns (runner method, assertion helpers, test structure). Call out anything that won't translate 1:1 and explain why.
- **Test data**: Which existing ISC/DISC configs will be reused, and whether new ones are needed.
- **Target file**: Where the new test will live and why (existing file vs new file).
- **Assertion coverage**: List every assertion from the e2e case and the corresponding integration check. Flag any assertions that will be dropped or changed, with justification.
- **Improvements**: Any improvements over the original (removing unnecessary polling, better assertions, broader coverage, etc.).

Do not proceed to implementation until the user approves the plan. If the user requests changes, revise and re-present.

### 5. Translate the test

Apply these conversions:

#### Registry setup
- **E2E**: Manual pod deployment via `createregistry()`, TLS trust via `trustCert()`, pull secret extraction.
- **Integration**: Use the suite's `testRegistry` global — it's started in `BeforeEach` and stopped in `AfterEach` automatically. No TLS or pull secret setup needed.

#### oc-mirror invocation
- **E2E**: `oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args(...)` with manual flag construction.
- **Integration**: Use `runner` methods from `tests/integration/pkg/ocmirror/`:
  - `runner.MirrorToMirror(ctx, iscPath, workDir, registryEndpoint, ...extraFlags)`
  - `runner.MirrorToDisk(ctx, iscPath, workDir, ...extraFlags)`
  - `runner.DiskToMirror(ctx, iscPath, workDir, registryEndpoint, ...extraFlags)`
  - `runner.DeletePhaseOne(ctx, discPath, workDir, registryEndpoint, ...extraFlags)`
  - `runner.DeletePhaseTwo(ctx, deleteYaml, registryEndpoint, ...extraFlags)`
  - `runner.ListOperators(ctx, ...args)`
  - `runner.ListReleases(ctx, ...args)`

#### Polling and retries
- **E2E**: Uses `wait.Poll()` and manual retry loops.
- **Integration**: Remove all polling — integration calls are synchronous. The runner returns when oc-mirror exits.

#### Assertions
- **E2E**: Raw `o.Expect(...)` on command output, manual image inspection.
- **Integration**: Use helpers from `tests/integration/helpers_test.go`:
  - `expectOcMirrorCommandSuccess(result, err)`
  - `expectSuccessfulMirrorInRegistry(iscPath, registry)`
  - `expectCorrectIDMS(workDir, iscPath)`
  - `expectValidDeleteImagesFiles(workDir, deleteID)`
  - `expectEmptyRegistry(registry)`
  - Follow the `expect*` naming pattern for any new helpers.

#### Test images
- **E2E**: Often uses real release images and production operator catalogs.
- **Integration**: Use the self-hosted test images:
  - Catalog: `quay.io/oc-mirror/oc-mirror-dev:test-catalog-latest`
  - Release: `quay.io/oc-mirror/release/test-release-index:v0.0.1`
  - Additional: `quay.io/openshifttest/hello-openshift@sha256:61b8f5...`

#### Test data configs
- Check existing ISC/DISC configs in `tests/integration/testdata/imagesetconfigs/` first.
- Only create new ones if the scenario needs a config that doesn't exist.
- Use test images (above) instead of production images.

### 6. Write the integration test

Follow the `automate-integration-test` skill for the writing conventions:
- Read `tests/integration/integration_suite_test.go` and `tests/integration/helpers_test.go` for current patterns
- Use Ginkgo v2 structure: `Describe` / `Context` / `It` / `By`
- Place the test in the appropriate existing file, or create a new one if it's a new category
- Every line should earn its place — no superfluous comments, unused variables, or boilerplate

### 7. Review the code

Before running, verify:
- Assertions actually test what the e2e case tested — don't lose coverage in the translation
- No e2e-specific patterns leaked in (manual polling, cluster assumptions, production images)
- Helpers are reused where possible

### 8. Run the test

Ask permission, then run:
```bash
PATH=$PATH:<path-to-registry-bin> go test -v ./tests/integration/ --ginkgo.focus "<test label>"
```

### 9. Recommend e2e disposition

After the integration test passes, advise the user:
- **Remove the e2e case** if it doesn't exercise anything cluster-specific (most common)
- **Keep the e2e case** if it tests cluster-level behavior (IDMS application, node reboot, operator deployment to a real cluster)
- **Keep both temporarily** if unsure, with a note to revisit
