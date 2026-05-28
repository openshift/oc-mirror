# oc-mirror Testing Strategy

## Overview

The goal of the `oc-mirror` testing strategy is to define a set of principles and guidelines that allows us to ensure the quality of `oc-mirror` in an efficient way. 

All the `oc-mirror` tests live in this repository and are executed in Prow.

---

## Testing Levels

### Unit Tests

Isolated checks that verify the correctness of individual functions or components, representing the smallest testable parts of the application.

> **Examples:**
> - Test the function that validates the format of the image reference string (e.g., checks for valid registry, repository, and tag).
> - Test the valid range of an integer flag.

### Low-level Integration Tests

Similar to the unit tests, these are still white-box tests, but instead of testing a single function they test the integration of more than one function.

> **Example:** Test that parsing an ImageSetConfiguration and then validating it correctly rejects a config where minVersion is greater than maxVersion.

### High-level Integration Tests

Tests that focus on verifying the core logic, behavior and external dependencies of the oc-mirror CLI by using a local registry.

These are black-box tests that do not have direct access to the `oc-mirror` internal code.

All the images used in these tests are built by us, in order to control their content and size.

In these tests we assert the tangible outputs of `oc-mirror`: archive contents, mirrored images present on the target registry, etc.

> **Example:** Execute the oc-mirror CLI command to successfully copy one image from a source registry to a local registry, and then verify the image exists in the local registry.

### End-to-end Tests

Tests that validate the complete end-to-end behavior of `oc-mirror` and its integration with OpenShift components by running on a fully provisioned OpenShift cluster, and mirroring realistic images.

Since E2E tests are expensive and slow, we should add them sparingly to cover critically important scenarios only. Anything else should be covered at the integration or unit levels.

> **Example:** Run the full oc-mirror process to mirror one image to a disconnected OpenShift cluster, then deploy a sample application using that mirrored image, and confirm the pod is running successfully on the cluster.

---

## Project Principles

| Principle                            | Description                                                                                                                                        |
|--------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------|
| 🤝 **Quality as a team responsibility** | Ensuring the quality of the project is a team effort. There is no single owner of quality.                                                         |
| 📦 **Self-contained PRs**               | Every pull request should include all the necessary tests at all the relevant testing levels. Reviewers should make sure that PRs are well covered. |
| 🤖 **Automation first**                 | All testing should be automated. A feature is not complete until it has all the necessary automated tests in place. Leverage AI to speed up generating and reviewing tests, while critically reviewing the agent's output. |
| 🎯 **Test at the right level**          | Don't implement an E2E test to check the range of a flag. Complex features might need tests at more than one level.                                |

---

## Principles for Efficient Testing

### ⬅️ Early testing/Shift-left

Catch defects as early as possible in the development cycle. The earlier a bug is found, the cheaper it is to fix.

### 📐 Testing Pyramid

Follow the testing pyramid: many unit tests at the base, fewer integration tests in the middle, and the fewest E2E tests at the top. Each level should test what it is best suited for — don't push scenarios up the pyramid unnecessarily.

```
                    /\
                   /  \
                  /    \
                 / E2E  \                 Few, slow, expensive
                /--------\
               /          \
              /            \
             / Integration  \             Moderate count and speed
            /----------------\
           /                  \
          /                    \
         /     Unit Tests       \         Many, fast, cheap
        /________________________\
```

### ⚠️ Risk-based testing

Prioritize testing efforts based on the risk and impact of failures. Features that are critical to users or have a history of bugs should get more coverage.
