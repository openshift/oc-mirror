# AI Agent Guide for oc-mirror

## Start Here

- Read [README.md](README.md) for an overview and [ARCHITECTURE.md](ARCHITECTURE.md) for the system design.
- Read [CONTRIBUTING.md](CONTRIBUTING.md) before making changes. Its contribution rules, development commands,
  and testing instructions apply to agents as well as human contributors.
- Consult [docs/](docs/) for feature documentation and the [testing strategy](docs/testing/README.md) for test guidance.
- After building with `make build`, use `./bin/oc-mirror --v2 --help` to inspect the CLI from this checkout.

## Task-Specific Skills

Use the applicable repository skills for your task, and read their instructions before starting the workflow.

| Task | Skill |
|------|-------|
| Fix a bug | [fix-bug](.agents/skills/fix-bug/SKILL.md) |
| Review code | [code-review](.agents/skills/code-review/SKILL.md) |
| Assess change risk | [assess-risk](.agents/skills/assess-risk/SKILL.md) |
| Write an integration test | [automate-integration-test](.agents/skills/automate-integration-test/SKILL.md) |
| Migrate an e2e test to integration | [migrate-e2e-to-integration](.agents/skills/migrate-e2e-to-integration/SKILL.md) |
| Analyze integration and e2e test coverage | [check-test-coverage](.agents/skills/check-test-coverage/SKILL.md) |

## Working Rules

1. Follow the repository's guidelines and workflows strictly by default. If asked to deviate, ask for confirmation
   before proceeding.
2. Keep changes focused and preserve unrelated work. Do not revert or discard changes you did not make.
3. Do not remove, skip, or weaken tests just to make checks pass.
4. Never include secrets, credentials, or sensitive information in commits, documentation, or shared output.
   If in doubt, ask the human.
5. Do not lie, cheat, fabricate results or make unreasonable assumptions.
