---
name: fix-bug
description: Workflow for analyzing, fixing and verifying bugs in `oc-mirror`
user-invocable: true
---

# Fixing Bugs in `oc-mirror`

A comprehensive workflow for analyzing, fixing, and verifying bugs in `oc-mirror`.

## 1. Analysis

Before writing any code, analyze the bug and identify its root cause. For this:

1. Review existing code of `oc-mirror` related to the bug.
2. Reproduce the bug you are trying to fix through the use of `oc-mirror` itself. Try to follow a similar approach
   to the integration tests in `tests/integration`: CLI + local registry + small images. If it is not possible to
   reproduce the bug due to the limitations of your environment, try to do a static reconstruction of the code
   path leading to the bug.
3. When relevant to understanding a non-trivial bug, review similar, recent bugs in `oc-mirror`. Gather information about
   up to 3 bugs going back up to 1 year in the past. Use GitHub and Jira if available. Read the conversations by
   humans about those bugs when possible, understand how the bugs were fixed. Consider if any of this information
   can be used in the present bug. Do not overdo this step, be mindful of your context.
4. Follow the [investigation guidelines](../../../AGENTS.md#investigation-and-research)
   to resolve remaining uncertainties about the bug.
5. Continue to the next step without asking for confirmation.

## 2. Design

1. Once the bug has been properly analyzed, propose a solution that addresses the root cause of the bug.
2. Do a self-review of your own solution:

   - Consider if it actually fixes the root cause.
   - Consider if it follows all the guidelines in CONTRIBUTING.md.
   - Consider alternative approaches that could be better (e.g. more robust, more secure, more efficient,
     more understandable), or if this bug should be fixed in one of our upstream dependencies.

3. Refine your solution based on the review.
4. Continue to the next step without asking for confirmation.

## 3. Implementation

Once the bug has been properly analyzed and a solid solution has been designed, make the corresponding changes
to the code.

1. Keep the changes focused and well-scoped.
2. Make sure that the changes are properly documented and tested at the right level (unit and/or integration
   via tests/integration).
3. Continue to the next step without asking for confirmation.

## 4. Review and Verification

Review the implementation:

1. Ensure that it actually fixes the root cause of the problem. Do this by running relevant tests, and if necessary
   by running extra manual tests using `oc-mirror`.
2. Use the [code-review skill](../code-review/SKILL.md) to run implementation and adversarial reviews, delegating to
   independent subagents when available and permitted, with the skill's fallback otherwise. Provide the bug's
   expected behavior, exact change scope, and investigation evidence already gathered; revisit the investigation
   only when evidence is missing or new questions arise.
3. Address validated findings and rerun relevant tests after changes. Request targeted re-review of affected
   areas when needed rather than restarting full reviews indefinitely. Use the summary format below rather than
   producing a separate review report; include any important unresolved concerns or review limitations.
4. Continue to the next step without asking for confirmation.

## 5. Summarize

Provide a short summary of the whole process and of the fix using simple language and a before/after comparison of the behavior with
real-world examples. Keep it short, 1-2 pages at most, preferably shorter.
