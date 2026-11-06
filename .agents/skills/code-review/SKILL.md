---
name: code-review
description: Workflow for reviewing code in `oc-mirror` using implementation and adversarial reviews
user-invocable: true
---
# Reviewing Code in `oc-mirror`

A comprehensive workflow for reviewing code in `oc-mirror`.
This skill can be used to review pull requests, local changes, or existing parts of the codebase.

## Identify the Code (Main Agent)

1. Determine what the code refers to, e.g.:
   - An explicit PR number/URL -> use `gh pr view <n> --json title,body,number,baseRefName,headRefName,commits,files`
    and `gh pr diff <n>` to retrieve the actual changes.
   - A branch already checked out -> `git status`, `git log --oneline -20`, diff against its merge-base with the base branch (usually `main`)
   - A raw diff/patch pasted or referenced by the user
2. Get the shape of the change before reading code:
   ```bash
     git diff <base>...<head> --stat
   ```
3. Read the actual diff, not just the statistics. For committed branch changes, use `git diff <base>...<head>`.
 For local working-tree changes, use `git diff` for unstaged changes and `git diff --cached` for staged changes.
 Also read relevant untracked files identified by `git status`.

Identify relevant staged, unstaged, and untracked files without treating unrelated local work as part of the change.

## Coordinate the Review (Main Agent)

1. Prepare the same factual brief for both reviewers: the scope established above, expected behavior, requirements, and relevant
   investigation evidence already available. Do not substitute the author's explanation for independent inspection.
2. When subagents are available and permitted, launch two in fresh contexts, in parallel when possible:
   - One follows **Implementation Review** below.
   - One follows **Adversarial Review** below.

   Give each the relevant instructions from this skill, including the summary format, and the exact references or
   files needed to inspect the scope. Both reviewers must independently read the actual diff and relevant files,
   including scoped staged, unstaged, and untracked changes, not just the brief or statistics.
   They must not invoke this coordination workflow or launch further reviewers.
3. Keep the review target unchanged until both reviewers finish. Reviewers must not edit the implementation;
   keep any safe, bounded test or reproduction artifacts isolated from the reviewed files and each other.
   Do not share one reviewer's initial conclusions with the other before both have returned findings.
4. If parallel execution is unavailable, use separate subagent sessions sequentially. If subagents are unavailable
   or not permitted, perform the implementation and adversarial reviews as separate passes yourself. Disclose the lack of
   independent review in the final summary.
5. Reconcile both sets of findings, perform targeted manual verification when warranted, and produce one final
   summary as described below.

## Implementation Review (Subagent)

Follow the existing workflow below for the assigned scope. Reuse investigation evidence from the brief;
resolve missing evidence or new questions independently. Return findings using the summary format below.

### Step 1: High-level Review

1. Analyze the purpose of the code and what's the user impact. Understand the "why" before the "how".
2. Follow the [investigation guidelines](../../../AGENTS.md#investigation-and-research)
   to gather the context needed for the review.
3. When reviewing a pull request, check the comments from humans and agents for additional context.
4. Continue to the next step without asking for confirmation.

### Step 2: Architectural Review

1. When relevant, trace the affected execution path and its integration with the surrounding components.
2. Analyze how the changes integrate with the rest of the codebase from an architectural point of view. Consider alternative approaches.
3. Focus on clean architectural patterns and simplicity. Consider if the changes are consistent with the rest of the codebase.
4. Continue to the next step without asking for confirmation.

### Step 3: Low-level Review

Now, it's time to review the actual code.

Things to look out for:

- For code changes, first of all consider if the code achieves its goal. For bugs, ensure that the fix actually fixes the bug; for features, ensure that the implementation fulfills the requirements of the feature.
- Overall code quality, security, and performance.
- Code smells, duplications, simplifications, idiomatic code and best practices.
- Consider if the same goal could be achieved in a simpler or more elegant way. Don't limit yourself to review the code and find issues, consider alternative approaches.
- Consider if anything could be achieved by using a dependency instead of reinventing the wheel.
- Consider if the proposed solution would make more sense as an upstream fix or feature (in one of the core dependencies).
- Look out for regressions and backwards-incompatible changes.
- Check if the test coverage is adequate.
- Consider if the architecture of the implementation is sane enough.

These are all things to look out for, you don't need to report on all of them. Focus on the important things and don't write big walls of text.

## Adversarial Review (Subagent)

Try to falsify the solution's claims: under what realistic conditions would it fail despite appearing correct
and passing its tests? Challenge assumptions rather than repeating the implementation review's code-quality checklist.

1. Identify the intended behavior and what must remain unchanged. For a bug fix, consider whether it addresses
   the root cause or only the reported example.
2. Identify assumptions about inputs, existing state, external services, ordering, permissions, and successful
   operations. Follow the [investigation guidelines](../../../AGENTS.md#investigation-and-research) and reuse
   available evidence; investigate further only where needed.
3. Construct realistic counterexamples and trace them through the actual code. Where useful and safe, attempt
   bounded reproductions. Focus on relevant risks, such as partial failures, interruption and retries, existing
   archives or configuration, differences between mirroring workflows, unintended deletion, trust violations,
   or success reported with incomplete results. Do not exercise destructive scenarios against real user data or services.
4. Check whether tests would detect these failures, or whether fixtures, mocks, and assertions assume away
   the behavior being challenged.
5. Return findings using the summary format below. Support them with triggering conditions, concrete impact,
   and code or test evidence. Separate unresolved concerns from confirmed failures. Do not invent objections
   or require a minimum number of findings; no supported findings is a valid outcome.

## Reconcile Results (Main Agent)

1. Deduplicate overlapping findings and verify important claims against code or tests. Resolve conflicting
   interpretations using evidence, not reviewer agreement or majority voting. A finding does not need both reviewers' support.
2. Exclude unsupported objections from confirmed findings, but retain important unresolved concerns with their
   uncertainties clearly stated. Do not blindly accept findings or dismiss them to defend the implementation.
3. Use the reconciled findings to guide targeted manual verification below. After verification, produce one
   consolidated summary using the summary format, rather than concatenating the reviewers' reports.

## Targeted Manual Verification (Main Agent)

Use the [investigation guidelines](../../../AGENTS.md#investigation-and-research) to scale this step. Run realistic
CLI scenarios for complex, non-trivial behavior or consequential failure paths, not for every review. Consider
risk rather than diff size. Reuse relevant investigation, reproduction, or reviewer test results already gathered
instead of repeating equivalent checks.

1. Select a small number of bounded scenarios based on the intended behavior, unresolved assumptions, and
   findings from both reviewers. Focus on behavior that code inspection and existing tests do not establish.
2. Use `make build` to build the code under review, then inspect `./bin/oc-mirror --v2 --help`.
   Follow the approach in [tests/integration](../../../tests/integration): CLI + local registry + small images,
   with an isolated workspace. Do not use real user data or run destructive operations against shared services.
3. Execute the relevant `oc-mirror` commands and verify tangible results, such as mirrored images, archive
   contents, or generated resources, not just exit status or success messages. For bug fixes, reproduce the
   failure before the change and verify corrected behavior afterward when feasible and safe, without disturbing
   the reviewed working tree.
4. Incorporate the results into the findings. Record the commands and configuration, expected versus observed
   behavior, and relevant limitations concisely. If execution is blocked, unsafe, or unnecessary, explain why;
   distinguish static reasoning from tests actually executed and do not claim unexecuted scenarios passed.

These ad hoc checks complement automated tests; they do not replace regression coverage or require adding a
permanent integration test during the review. Report missing regression coverage separately when relevant.

## Summary Format

The output of this skill should be a list of findings related to the review above. They should be categorized in Major, Medium, and Minor.
For each finding provide:

- Very short summary, simple language, concise.
- File and line number.
- Triggering conditions and concrete impact, supported by code or test evidence. Distinguish confirmed issues
from uncertainties and optional suggestions.
- Relevant snippet of code.
- Suggested fix when it makes sense, with the corresponding code snippet.
- Whether this finding is a result of the changes being reviewed or it was pre-existing.
- Highlight any changes that break backwards compatibility.

Prioritize findings that could cause REAL, IMPACTFUL ISSUES. We don't need to have issues in all three categories. Focus on the pragmatic, impactful approaches.

If there are no supported findings, say so. Briefly disclose review or verification limitations, including whether
the two reviews ran in independent subagent sessions.
