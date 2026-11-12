# AI Agent Guide for oc-mirror

## Start Here

- Read [README.md](README.md) for an overview and [ARCHITECTURE.md](ARCHITECTURE.md) for the system design.
- Read [CONTRIBUTING.md](CONTRIBUTING.md) before making changes. Its contribution rules, development commands,
  and testing instructions apply to agents as well as human contributors.
- Consult [docs/](docs/) for feature documentation and the [testing strategy](docs/testing/README.md) for test guidance.
- After building with `make build`, use `./bin/oc-mirror --v2 --help` to inspect the CLI from this checkout.

## Task-Specific Skills

This repository has skills that must be used by agents when performing certain tasks:

| Task | Skill |
|------|-------|
| Fix a bug | [fix-bug](.agents/skills/fix-bug/SKILL.md) |
| Review code | [code-review](.agents/skills/code-review/SKILL.md) |
| Write an integration test | [automate-integration-test](.agents/skills/automate-integration-test/SKILL.md) |
| Migrate an e2e test to integration | [migrate-e2e-to-integration](.agents/skills/migrate-e2e-to-integration/SKILL.md) |
| Analyze integration and e2e test coverage | [check-test-coverage](.agents/skills/check-test-coverage/SKILL.md) |

## Investigation and Research

1. Base conclusions on relevant code and evidence rather than assumptions. When needed, consult official documentation,
   dependency source code, or targeted web research to resolve uncertainties.
2. For non-trivial changes or code reviews, consult relevant repository history, related bugs or features (in Jira, if possible), and the human discussions
   around them to understand previous decisions and fixes.
3. For non-trivial changes or code reviews, check relevant open PRs for existing fixes, overlapping changes, and related
   design discussions. Keep the search focused on the affected behavior or components, and inspect relevant diffs and
   discussions rather than relying on titles or file overlap alone. Treat unmerged PRs as context, not established requirements
   or proof of correctness.
4. Scale investigation to the complexity and impact of the change. Avoid broad research for simple changes, and reuse
   evidence already gathered unless new questions or missing evidence require further investigation.

## Working Rules

1. Follow the repository's guidelines and workflows strictly by default. If asked to deviate, ask for confirmation
   before proceeding.
2. Keep changes focused and preserve unrelated work. Do not revert or discard changes you did not make.
3. Do not remove, skip, or weaken tests just to make checks pass.
4. Never include secrets, credentials, or sensitive information in commits, documentation, or shared output.
   If in doubt, ask the human.
5. Do not lie, cheat, fabricate results or make unreasonable assumptions.
