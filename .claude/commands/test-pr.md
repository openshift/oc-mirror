# Test PR - Analyze PR Changes and Plan Testing

Analyze a GitHub Pull Request and help plan testing strategy based on the changes.

## Instructions

You are helping the user analyze a pull request and determine the appropriate testing approach. Follow these steps to provide comprehensive testing guidance.

### Step 1: Get PR Information

Ask the user for the PR number if not provided as an argument. The PR link should be in one of these formats:
- Full URL: `https://github.com/owner/repo/pull/123` (extract the number)
- Short format: `owner/repo#123` (extract the number)
- Just number: `123` (if in current repo)

### Step 2: Fetch and Analyze PR Changes

Use `git` to fetch and analyze PR changes (no `gh` CLI or extra dependencies required):

```bash
# Determine the base branch (usually main or master)
BASE_BRANCH=$(git symbolic-ref refs/remotes/origin/HEAD | sed 's@^refs/remotes/origin/@@')

# Fetch the PR branch from the remote
git fetch origin pull/<PR_NUMBER>/head:pr-<PR_NUMBER>

# View PR commits (title and description from commit messages)
git log ${BASE_BRANCH}..pr-<PR_NUMBER> --format="%h %s%n%b" --no-merges

# Get the diff (list of changed files with stats)
git diff ${BASE_BRANCH}...pr-<PR_NUMBER> --stat

# Get the actual diff content
git diff ${BASE_BRANCH}...pr-<PR_NUMBER>
```

**Note:** This approach uses `git fetch origin pull/<PR_NUMBER>/head` which works with GitHub-hosted remotes without requiring any extra CLI tools. After analysis, clean up the local ref with `git branch -D pr-<PR_NUMBER>`.

Analyze the changes to understand:
- **Files modified** - Which files and directories were changed
- **Scope of changes** - New features, bug fixes, refactoring, etc.
- **Code patterns** - Functions added/modified, API changes, config changes
- **Test files** - Whether any test files were already included
- **Documentation** - README, docs, comments updates

### Step 3: Ask About Test Type Requirements

Use the AskUserQuestion tool to ask which types of tests are required. Present these options:

**Test Type Options:**
1. **Unit Tests** - Test individual functions and methods in isolation
2. **Integration Tests** - Test interaction between components/modules
3. **E2E Tests** - Test complete workflows from end to end
4. **Regression Tests** - Verify existing functionality still works
5. **Manual Tests** - Human-driven testing with specific test cases
6. **Ad-hoc Tests** - Exploratory testing without formal test cases
7. **Exploratory Tests** - Unstructured investigation of the changes

Allow the user to select multiple test types that are needed.

### Step 4: Detailed Analysis Based on Selection

Based on the selected test types, provide specific guidance:

#### For Unit Tests:

1. **Identify testable functions** - Scan the diff for:
   - New functions/methods added
   - Modified functions that changed behavior
   - Functions with complex logic (conditionals, loops, error handling)

2. **Present function options** - Use AskUserQuestion to show:
   - List of functions that need tests (with file path and line number)
   - Brief description of what each function does
   - Current test coverage status (if tests exist)

3. **Prioritize testing order** - After user selects, explain:
   - Why this function is important to test first
   - Edge cases to consider
   - Mock/stub requirements

4. **Provide test steps:**
   ```bash
   # Run unit tests for specific package
   go test -v ./pkg/path/to/package -run TestFunctionName

   # Run with coverage
   go test -v -coverprofile=coverage.out ./pkg/path/to/package
   go tool cover -html=coverage.out
   ```

#### For Integration Tests:

1. **Identify integration points** - Scan for:
   - API calls between modules
   - Database interactions
   - External service dependencies
   - Component interactions

2. **Present integration scenarios** - Use AskUserQuestion to show:
   - List of integration points that need testing
   - Components involved in each scenario
   - Data flow and dependencies

3. **Provide test steps:**
   ```bash
   # Run integration tests
   make integration

   # Or run specific integration test suite
   go test -v ./test/integration -run TestIntegrationScenario
   ```

#### For E2E Tests:

1. **Identify user workflows** - Scan for:
   - CLI command changes
   - API endpoint modifications
   - Configuration file handling
   - Complete user journeys affected

2. **Present workflow options** - Use AskUserQuestion to show:
   - List of end-to-end workflows that need testing
   - Prerequisites for each workflow
   - Expected inputs and outputs

3. **Provide test steps:**
   ```bash
   # Run e2e tests
   make e2e

   # Or run specific e2e scenario
   ./test/e2e/run-scenario.sh <scenario-name>

   # For oc-mirror specifically
   ./bin/oc-mirror -c test-config.yaml file:///tmp/test-output --v2
   ```

#### For Regression Tests:

1. **Identify affected areas** - Scan for:
   - Modified functions that are called by other code
   - Changed interfaces or APIs
   - Refactored code paths

2. **List regression test suites:**
   ```bash
   # Run full test suite
   make test

   # Run tests for affected packages
   go test -v ./pkg/... -run "TestExisting.*"
   ```

#### For Manual Tests:

1. **Create test cases** - Based on changes, generate:
   - Step-by-step test procedures
   - Test data requirements
   - Expected results
   - Screenshots or log verification steps

2. **Provide test checklist:**
   - [ ] Test case 1: Description
   - [ ] Test case 2: Description
   - [ ] Verify logs/output
   - [ ] Check error handling

#### For Ad-hoc/Exploratory Tests:

1. **Provide exploration guidelines:**
   - Areas to focus on based on changes
   - Boundary conditions to try
   - Error scenarios to trigger
   - Performance characteristics to observe

2. **Suggest test commands:**
   ```bash
   # Try different inputs
   ./bin/oc-mirror <various-flags-and-configs>

   # Test error conditions
   ./bin/oc-mirror -c invalid-config.yaml ...

   # Observe behavior under load
   time ./bin/oc-mirror ...
   ```

### Step 5: Detailed Testing Steps

After the user selects a specific function or scenario to test first, provide:

1. **Test file location** - Where to create/modify the test
   ```
   File: pkg/path/to/package/file_test.go
   ```

2. **Test template** - Provide a starter test function:
   ```go
   func TestFunctionName(t *testing.T) {
       tests := []struct {
           name    string
           input   InputType
           want    ExpectedType
           wantErr bool
       }{
           {
               name: "success case",
               input: InputType{...},
               want: ExpectedType{...},
               wantErr: false,
           },
           {
               name: "error case",
               input: InputType{...},
               wantErr: true,
           },
       }

       for _, tt := range tests {
           t.Run(tt.name, func(t *testing.T) {
               got, err := FunctionName(tt.input)
               if (err != nil) != tt.wantErr {
                   t.Errorf("FunctionName() error = %v, wantErr %v", err, tt.wantErr)
                   return
               }
               if !reflect.DeepEqual(got, tt.want) {
                   t.Errorf("FunctionName() = %v, want %v", got, tt.want)
               }
           })
       }
   }
   ```

3. **Run commands** - Specific commands to execute the test:
   ```bash
   # Run the specific test
   go test -v ./pkg/path/to/package -run TestFunctionName

   # Run with verbose output
   go test -v -count=1 ./pkg/path/to/package -run TestFunctionName

   # Run with coverage
   go test -v -coverprofile=coverage.out ./pkg/path/to/package -run TestFunctionName
   go tool cover -func=coverage.out
   ```

4. **Validation steps** - How to verify the test passes:
   - Check test output shows PASS
   - Verify all sub-tests passed
   - Review coverage percentage
   - Check for race conditions: `go test -race ...`

5. **After providing the above** - Proceed to Step 7 to ask what the user wants to do next (test another function, move to another test type, or finish)

### Step 6: Additional Guidance

Provide context-specific advice:

- **For oc-mirror**: Point to existing test patterns in `test/` directory
- **Test fixtures**: Where to place test data files
- **Mocking**: How to mock external dependencies
- **CI integration**: How these tests run in the pipeline
- **Test naming**: Conventions for test function names
- **Coverage goals**: Target coverage percentage

### Step 7: Loop Back for Remaining Work

After completing the detailed testing steps for one function/scenario, ALWAYS ask the user what they want to do next using AskUserQuestion:

**Present these options:**

1. **Test another function** (for the current test type) - If there are remaining functions in the same test type that haven't been covered yet
   - Example: "We just covered pinCatalogDigests(). Would you like to test pinSingleCatalogDigest() next?"

2. **Move to another test type** - If there are other selected test types that haven't been addressed yet
   - Example: "We've completed the Unit Tests section. Would you like to move to Integration Tests now?"

3. **Test a different scenario** (for E2E/Integration tests) - If working on E2E or integration tests with multiple workflows
   - Example: "We've covered M2D scenario. Would you like to test D2M using pinned ISC next?"

4. **Done with testing** - User is satisfied with the guidance provided

**Implementation:**

Use AskUserQuestion after providing detailed steps (Step 5) to ask:
```
Question: "What would you like to do next?"
Options:
- Continue with remaining functions/scenarios for [current test type]
- Move to [next uncovered test type]
- Review test coverage and summary
- Done with test planning
```

**Important:**
- Track which test types were originally selected
- Track which functions/scenarios have been covered
- Only offer options for uncovered work
- If all selected test types are covered, offer summary or completion
- DO NOT write actual test code unless user explicitly requests it - provide templates and guidance instead

## Important Notes

- Only use `git` commands to fetch and analyze PR changes - never rely on `gh` CLI or other external tools
- Always fetch the actual PR diff to understand changes, don't make assumptions
- Prioritize tests based on risk and complexity of changes
- For functions with existing tests, suggest running them first to catch regressions
- Consider the test pyramid: more unit tests, fewer integration tests, even fewer e2e tests
- Provide runnable commands that work in the project's actual test infrastructure
- Reference existing test files as examples when available
- Use AskUserQuestion for interactive selection, not just listing options
- If PR includes test files, review them and suggest improvements

## Example Usage

```
User: /test-pr https://github.com/openshift/oc-mirror/pull/1234

Claude: I'll analyze PR #1234 for testing requirements.
[Fetches PR details]
[Analyzes changes]

Claude: This PR modifies the mirror workflow and adds a new validation function.
        Which types of tests do you need?
        [Presents multiple-choice options]

User: Unit and E2E tests

Claude: For unit tests, I found these functions that need testing:
        1. ValidateImageSetConfig() in pkg/config/validate.go:45
        2. ProcessManifest() in pkg/mirror/manifest.go:123
        3. CalculateDigest() in pkg/image/digest.go:78

        Which function should we test first?

User: ValidateImageSetConfig

Claude: Great choice - this is a critical validation function.
        [Provides test template, run commands, and detailed steps]

        What would you like to do next?
        [Shows options: test another function, move to E2E tests, done]

User: Test another function

Claude: Which function should we test next?
        1. ProcessManifest() in pkg/mirror/manifest.go:123
        2. CalculateDigest() in pkg/image/digest.go:78

User: ProcessManifest

Claude: [Provides test template, run commands, and detailed steps for ProcessManifest]

        What would you like to do next?
        [Shows options: test CalculateDigest, move to E2E tests, done]

User: Move to E2E tests

Claude: For E2E tests, these workflows need testing:
        1. M2D workflow - verify end-to-end mirroring
        2. D2M workflow - verify disk to mirror
        3. M2M workflow - verify mirror to mirror

        Which workflow should we test first?

[Process continues...]
```
