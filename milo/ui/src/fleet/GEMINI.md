# LUCI Fleet UI Rules

This document provides design guidelines and rules for AI assistants working on the LUCI Fleet Console UI.

For a general project overview and local setup, see [README.md](./README.md).

## Style Guide

### Avoid `any`
Do not use `any` in TypeScript without explicit permission. Use strong types or generics instead (e.g., avoid `MyType<any>`).

## AI Agent Workflow Rules

### 1. Mandatory Verification
For code changes, add verification steps to your `task.md` checklist. Before declaring a task done, run:
- **Linting**: `npm run lint` (or `npm run lint-inc` to lint files changed against `origin/main`). Use `npm run lint -- --fix <path>` to auto-fix styling.
- **Testing**: `npm test -- <path_to_test_file>` (or `npm test -- ./src/fleet` for all Fleet tests).
- **Type Checking**: `npm run type-check`.

### 2. Definition of Done
A task or frontend CL is complete when:
- **Self-Review**: You run [senior-reviewer](./.agents/skills/senior-reviewer/SKILL.md) and resolve all critical feedback.
- **UX & PM Review**: For visual or flow changes, you run [ux-pm-review](./.agents/skills/ux-pm-review/SKILL.md) to check PM alignment, Orwell writing rules, and UX principles.
- **Verification**: Tests, lints (`npm run lint`), and type checks (`npm run type-check`) pass cleanly.
- **UI Demo**: You upload a demo for visual or structural UI changes and include testing steps in the commit message.
- **Commit Message**: Explains the change and references relevant bugs (see [prepare-cl](./.agents/skills/prepare-cl/SKILL.md)).
- **Direct Upload**: Upload the CL to Gerrit directly via `git cl upload`.

### 3. Self-Review & UX/PM Audits
- Run [senior-reviewer](./.agents/skills/senior-reviewer/SKILL.md) to review diffs before uploading CLs.
- For major UI features, run [design-tournament](./.agents/skills/design-tournament/SKILL.md) to compare layout options.
- Audit copy and layouts against [writing-and-ux-principles](./.agents/skills/writing-and-ux-principles/SKILL.md) (Orwell writing rules, Material writing spec, and Laws of UX).

### 4. Coding Conventions
- Rely on TypeScript type inference and narrowing instead of type casting (`as Type`).

### 5. Temporary File Hygiene
- Do not run `rm` commands to delete temporary files.
- Store transient test outputs in the gitignored `.tmp/` directory.
- Overwrite existing files with empty strings (`""`) to clear disk space without permission prompts.

### 6. Proto Generation
Do not run root `npm run gen-proto`. Run the Fleet-specific script from `milo/ui/`:
```sh
bash src/fleet/gen_ts_proto.sh
```

## Architectural Principles & Decisions
Architecture decision records live in `docs/decisions/`:
- **Keep Status Current**: Update decision docs as migrations progress to reflect current technical status.

## Available Skills
Detailed procedural workflows live in [.agents/skills/](./.agents/skills/):
- [prepare-cl](./.agents/skills/prepare-cl/SKILL.md)
- [senior-reviewer](./.agents/skills/senior-reviewer/SKILL.md)
- [ux-pm-review](./.agents/skills/ux-pm-review/SKILL.md)
- [design-tournament](./.agents/skills/design-tournament/SKILL.md)
- [writing-and-ux-principles](./.agents/skills/writing-and-ux-principles/SKILL.md)
- [gerrit-workflows](./.agents/skills/gerrit-workflows/SKILL.md)
- [high-density-ui](./.agents/skills/high-density-ui/SKILL.md)

Shared repository skills live in [../../../../.agents/skills](../../../../.agents/skills).

## Confidentiality
This project is open source:
- Do not leak internal confidential details, private URLs, or credentials.
- Ensure `go/` link titles do not expose sensitive project names.

## 7. Gerrit Upload Safety
Follow [gerrit-workflows](./.agents/skills/gerrit-workflows/SKILL.md) before creating branches or running `git cl upload`:
1. Branch from `origin/main`.
2. Verify local commits before upload: `git log origin/main..HEAD --oneline`.
3. Check CL issue association with `git cl issue`.
