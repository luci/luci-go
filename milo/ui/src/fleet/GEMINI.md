# Gemini Code Assist - LUCI Fleet UI Rules

This document provides the active design guidelines and rules for AI code assistants like Gemini working on the LUCI Fleet Console UI subproject.

For general project overview, architecture, and local setup instructions, see the [README.md](./README.md).

# Style guide

## Avoid any
Do not ever use `any` in typescript code without permission. You should try very hard to avoid it all together but if you truly think that it is necessary get explicit permission before using it. This includes usage of any inside generics IE: `MyType<any>`.

# AI Agent Workflow Rules

## 1. Mandatory Verification
For every task that involves code changes, the agent MUST explicitly add tasks to its `task.md` checklist for running lints, tests, and type-checks. Before declaring any task as "done," you MUST run the following verification suite and complete those tasks. If any step fails, you must fix the error and re-run the suite until it passes.

- **Linting:** Run `npm run lint` to verify linting across the project, or `npm run lint-inc` to quickly lint only files changed against `origin/main`. Use `npm run lint -- --fix <path>` if you need to auto-fix a specific file.
- **Testing:** Run tests related to your changes using `npm test -- <path_to_test_file>`. To run all Fleet tests, use `npm test -- ./src/fleet`.
- **Type Checking:** Run `npm run type-check` to ensure no typing regressions were introduced.

## 2. Definition of Done (Standard Workflow)
A task or frontend CL is not considered complete until:
- **Self-Review**: The [senior-reviewer](./.agents/skills/senior-reviewer/SKILL.md) workflow has been run and no critical feedback remains.
- **Verification**: All tests, lints (`npm run lint`), and type-checks (`npm run type-check`) pass successfully.
- **UI Demo**: A demo has been uploaded (required for any change that adds, removes, or structurally alters visual components, pages, or user flows) and testing steps are included in the commit message.
- **Commit Message**: The commit message clearly explains the change. If the change fixes a bug, it should reference it in the proper format (see [prepare-cl](./.agents/skills/prepare-cl/SKILL.md) skill for details).
- **Direct Upload**: The agent should attempt to directly upload the CL and handle presubmits when possible (respecting sandbox limits as detailed in the [prepare-cl](./.agents/skills/prepare-cl/SKILL.md) skill documentation).

**Failure to follow this workflow results in unnecessary round trips. Following this process is part of the task.**

## 3. Mandatory Self-Review via Subagent
For every task that involves code changes, the agent MUST use the [senior-reviewer](./.agents/skills/senior-reviewer/SKILL.md) skill to perform a self-review of the diff and address all feedback before declaring the task complete or uploading a CL.

## 4. Coding Conventions & Best Practices
- **Avoid type casting unless strictly necessary.** Try to rely on TypeScript's type inference and narrowing instead of using `as Type`.

## 5. Sandbox & Temp File Hygiene (No terminal deletion prompts)
To maximize productivity and prevent unnecessary manual permission prompts for the user:
- **Do NOT run `rm` or other terminal deletion commands** to clean up temporary/sandbox files, patches, or log dumps.
- **Leave transient files in `.tmp/`**: Since `.tmp/` is globally gitignored and excluded from all build tools, leaving files there is completely safe and causes no workspace pollution.
- **Overwrite instead of delete**: If you need to clear a file's content or size to save disk space, use the `write_to_file` tool to overwrite the file with an empty string (`""`) rather than running `rm`.

## Architectural Principles & Design Documentation
We maintain documentation of key architectural principles and design tradeoffs in `decisions/` directories.
- Frontend-specific and cross-cutting docs live in `./docs/decisions/`
- Backend-specific docs live in `../../../../../../infra/fleetconsole/decisions/`
- **Keep migration status current:** As you make progress on migrations (like the AIP-160 transition), please update the relevant decision documents to reflect the current technical status quo and future intent to avoid confusion / regressions when migrations are in transition states.

## Available Skills
To avoid context bloat, detailed procedural knowledge and domain-specific instructions are extracted into **Skills**. The harness loads these skills on-demand based on your task. Skills follow the open standard defined at [agentskills.io](https://agentskills.io/home). You can find available skills in:
- `src/fleet/.agents/skills/`

Available skills include:
- [prepare-cl](./.agents/skills/prepare-cl/SKILL.md)
- [senior-reviewer](./.agents/skills/senior-reviewer/SKILL.md)
- [manual-testing](./.agents/skills/manual-testing/SKILL.md)
- [aip160-filtering](./.agents/skills/aip160-filtering/SKILL.md)
- [high-density-ui](./.agents/skills/high-density-ui/SKILL.md)
- [project-verification](./.agents/skills/project-verification/SKILL.md)
- [ux-prototyping](./.agents/skills/ux-prototyping/SKILL.md)
- [preventing-workspace-leakage](./.agents/skills/preventing-workspace-leakage/SKILL.md)
- [managing-parallel-workspaces](./.agents/skills/managing-parallel-workspaces/SKILL.md)
- [bypassing-interactive-prompts](./.agents/skills/bypassing-interactive-prompts/SKILL.md)
- [continuous-improvement](./.agents/skills/continuous-improvement/SKILL.md)


## Confidentiality Guidelines
This project is open source. When writing code, documentation, or commit messages:
- **DO NOT** leak internal confidential information.
- **DO NOT** include sensitive server names, non-public URLs, or internal credentials in code or docs.
- `go/` links are allowed, but the link text itself (e.g., the short link name) **must not** contain confidential information.
- Redact or use placeholders for sensitive details if necessary.
