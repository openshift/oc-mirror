---
argument-hint: <pr-url>
description: Analyze a GitHub Pull Request and provide QE testing approach and recommendations (line numbers only, no code)
model: sonnet
---

# 🧪 AI PR Testing Strategy Analyzer

**Allowed Tools**: `Bash(gh:*)`, `Bash(git:*)`
**Target PR**: `$ARGUMENTS`

---

## 📋 Step 1: Parse PR Information

Extract the PR details from the provided GitHub PR URL:

- **Expected format**: `https://github.com/owner/repo/pull/number`
- **Example**: `https://github.com/openshift/oc-mirror/pull/1284`

The `gh` CLI tool accepts full PR URLs directly, making parsing straightforward.

```bash
!gh pr view $ARGUMENTS --json number,title,author,headRefName,files
```

---

## 🔍 Step 2: Fetch PR Diff

Get the detailed diff for the PR to understand what changed:

```bash
!gh pr diff $ARGUMENTS
```

**IMPORTANT**: Extract and display ONLY the line numbers that were changed from the diff. DO NOT show the actual code content. Parse the diff to identify:
- Files changed
- Line numbers added (marked with +)
- Line numbers removed (marked with -)
- Line ranges modified

Present this as: `filename:line_number` or `filename:start_line-end_line`

---

## 🔬 Step 3: Analyze Changes

Based on the PR diff line numbers and metadata, analyze the following:

### 3.1 Language/Framework Detection

Identify the primary programming language and testing framework used in the project.

### 3.2 Change Type Analysis

Determine the nature of the change:

| Change Type       | QE Testing Requirement                                           |
|-------------------|------------------------------------------------------------------|
| **New feature**   | Integration, E2E, and manual testing with happy path + edge cases |
| **Bug fix**       | Regression testing targeting the specific bug and related workflows |
| **Refactor**      | Regression and integration testing ensuring behavior unchanged    |
| **Performance**   | Performance, load, and benchmark testing                         |
| **Documentation** | Minimal/no testing needed                                        |
| **Test-only**     | No additional QE tests needed                                    |

### 3.3 File Classification

For each file in the PR, categorize:

- ✅ **Source files** (need testing)
- 🧪 **Test files** (already tests)
- ⚙️ **Config/documentation files** (skip)
- 📦 **Generated/binary files** (skip)

---

## 🎯 Step 4: Identify Testing Needs

For each modified source file, identify:

### 4.1 Code Change Analysis

Understand from the diff:

- What functions/methods were added or modified
- What the expected behavior is
- What edge cases exist
- What errors could occur
- What dependencies are involved

### 4.2 QE Test Coverage Recommendations

Recommend what should be tested from a QE perspective:

#### For Bug Fixes

- Regression test scenarios that reproduce the original bug
- End-to-end verification that the fix works in production-like environment
- Related edge cases and user workflows
- Impact on existing functionality

#### For New Features

- Integration testing with existing features
- End-to-end user workflows with valid inputs
- Edge cases (empty, null, boundary values)
- Error conditions (invalid inputs, system failures)
- Performance and load testing if applicable

#### For Refactors

- Functional testing ensuring behavior hasn't changed
- Integration testing with dependent components
- Performance and regression testing

### 4.3 Suggested Test Scenarios

List the specific test scenarios that should be covered, including:

- Test scenario descriptions (what to verify)
- Edge cases to consider
- Mock/stub requirements

**DO NOT provide code examples or test function implementations**

---

## 📊 Step 5: Present Testing Strategy

Display a comprehensive testing approach summary with the following sections:

### PR Overview

- PR number, title, and author
- Change type (feature, bug fix, refactor, etc.)
- Files modified and their purpose
- Language and testing framework detected

### Changed Lines Summary

**CRITICAL**: Display ONLY the line numbers that were modified, NOT the code content:

For each file, show:
- File path
- Line numbers added: `filename:line_number` format
- Line numbers removed: `filename:line_number` format
- Line ranges modified: `filename:start_line-end_line` format

Example format:
```
src/handler.go:45-67 (modified)
src/handler.go:102 (added)
src/handler.go:89-91 (removed)
```

### Change Analysis

For each modified source file:

- High-level description of what changed (NO code snippets)
- Impact and scope of changes
- Dependencies and integrations affected
- Reference to line numbers from Changed Lines Summary above

### QE Testing Recommendations

#### Test Scenarios by Changed Files

For each file that needs testing, provide:

- Suggested test scenarios (list 5-10 specific descriptions of what to test from QE perspective)
- Edge cases to cover
- Error conditions to handle
- Integration points to verify
- User workflows affected

**DO NOT include any code examples or test implementations**

#### Test Strategy

- Test levels (integration, e2e, manual, regression, performance)
- Coverage goals
- Priority areas (critical paths first)
- Risk assessment

#### Test Scenario Descriptions

Provide descriptive test scenario descriptions like:

- "Verify end-to-end workflow with valid user input"
- "Test system behavior with null/empty input"
- "Test system behavior with boundary values"
- "Verify error handling when external dependency fails"
- "Test integration with dependent services/components"
- "Verify backward compatibility with existing features"

**DO NOT write actual test code or function names**

### QE Testing Approach Guidance

#### Test Environment Considerations

- Required test environment setup
- Test data requirements
- External dependencies and integrations
- Configuration needed for testing

### Files Not Requiring Tests

List files that don't need testing and why:

- Test files (already tests)
- Configuration files
- Documentation
- Minor changes covered by existing tests

---

**Note**: Present ONLY the QE testing approach and strategy from a quality engineering perspective. Focus on integration, e2e, regression, and manual testing scenarios. DO NOT include any code examples, snippets, test implementations, or unit test recommendations. Reference changes by line numbers only.

