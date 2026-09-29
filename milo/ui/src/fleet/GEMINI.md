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

### 2. Definition of Done & 3-Second Visual Review Package
A task or frontend CL (including autonomous 1-CL fixes) is complete when:
- **Self-Review**: You run [senior-reviewer](./.agents/skills/senior-reviewer/SKILL.md) and resolve all critical feedback.
- **UX & PM Review**: For visual or flow changes, you run [ux-pm-review](./.agents/skills/ux-pm-review/SKILL.md) to check PM alignment, Orwell writing rules, and UX principles.
- **Verification**: Tests, lints (`npm run lint`), and type checks (`npm run type-check`) pass cleanly with 0 errors and 0 warnings.
- **Visual Proof Front-and-Center (`Screenshots:` & `Demo:` First)**: For any UI or visual change, place `Screenshots:` (with an interactive **Before/After Overlay Toggle** link `http://go/zhangtiff-labs/before-after?before=<before_id>&after=<after_id>` plus full-resolution `Before:` and `After:` links) and `Demo:` (a real App Engine `luci-milo-dev` URL via [deploy-ui-demo](./.agents/skills/deploy-ui-demo/SKILL.md)) **at the very top of the CL description immediately after the title line**. When a visual CL spans multiple pages or workflows, include `Before` and `After` screenshots for **each major workflow that changed**. Googlers can upload screenshots using `http://go/screenshot` (`snipit`), while external / open-source contributors can attach screenshots on the linked issue tracker (`b/...`) and link them in `Screenshots:`.
- **Plain-English `TL;DR / Why:` (Zero Code Jargon)**: Write a 1-2 sentence plain-English `TL;DR / Why:` explaining what visual problem users see today and how the change makes the UI cleaner or easier to scan—without citing `.tsx` filenames, CSS properties, or hook names. Place technical implementation details in `Context:` below (see [prepare-cl](./.agents/skills/prepare-cl/SKILL.md)).
- **Direct Upload**: Upload the CL to Gerrit directly via `git cl upload`.

### 2b. When to Use an App Engine Live Demo (`luci-milo-dev`) vs. a Static Easy Mock Prototype

| Workflow Path | When to Use | Deliverable & Verification Package |
| :--- | :--- | :--- |
| **App Engine Live Demo (`luci-milo-dev.appspot.com`) + Interactive Before/After Screenshots** *(Preferred whenever code is being added live to prod)* | All Gerrit CLs adding or updating live production UI code—including small UX fixes, visual polish/beautification, chip/typography formatting, new columns/filters, bug fixes, and autonomous 1-CL fixes (`<= 150` prod LOC). | Standard Gerrit CL on `luci-go` with: (1) Interactive Before/After overlay toggle (`http://go/zhangtiff-labs/before-after?before=<id>&after=<id>`) + individual `Before`/`After` screenshots for each affected page/workflow and a deep-linked `https://<short-version>-dot-luci-milo-dev.appspot.com/ui/fleet/...` live demo at the very top of the description, and (2) plain-English `TL;DR / Why:` + `Context:`. Never substitute a static Easy Mock link for an App Engine demo on a production CL. |
| **Static Easy Mock Prototype ([ux-prototyping](./.agents/skills/ux-prototyping/SKILL.md))** *(Strictly for longer-term ideas not yet at the CL stage)* | Longer-term product ideas that are **not yet at the CL stage**: early PM/UX ideation, multi-option [design-tournament](./.agents/skills/design-tournament/SKILL.md) explorations, or net-new multi-page concepts requiring mocked backend RPCs that do not exist yet. | Self-contained static mock bundle with client-side mock pRPC fixtures (`settings.js`) for early stakeholder exploration before writing production CLs. |

### 3. Self-Review & UX/PM Audits
- Run [senior-reviewer](./.agents/skills/senior-reviewer/SKILL.md) to review diffs before uploading CLs.
- For major UI features, run [design-tournament](./.agents/skills/design-tournament/SKILL.md) to compare layout options.
- Audit copy and layouts against [writing-and-ux-principles](./.agents/skills/writing-and-ux-principles/SKILL.md) (Orwell writing rules, Material writing spec, and Laws of UX).

### 4. Coding Conventions
- Rely on TypeScript type inference and narrowing instead of type casting (`as Type`).
- **Legacy Naming Conventions (`_OLD`)**: Do not copy or extend files or exports with `_OLD` / `_deprecated` naming suffixes.
- **Reuse Shared Components & Utilities**: Respect the domain folder organization in `src/fleet/components/` and `src/fleet/utils/`. Before creating new UI widgets, dialogs, or formatting helpers, scan `src/fleet/components/` and `src/fleet/utils/` for existing implementations to reuse or extend (see [components/README.md](./components/README.md)).

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
- **Cross-Link Core Framework Guides**: Avoid duplicating core LUCI Milo UI framework documentation. Link to framework guides using relative Markdown links:
  - Feature Flags: [`../../docs/guides/using_feature_flags.md`](../../docs/guides/using_feature_flags.md)
  - Authentication & Authorization: [`../../docs/guides/authentication_and_authorization.md`](../../docs/guides/authentication_and_authorization.md)
  - Pagination: [`../../docs/guides/effective_pagination.md`](../../docs/guides/effective_pagination.md)

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

## 8. Shared Infrastructure Protection
When working on Fleet Console code:
- **Do not modify shared layout guards**: Never remove environment checks in `src/common/` or shared app shells (`Header.tsx`, `fleet_layout.tsx`).
- **Use `allowedEnvironments` for feature flags**: Ensure feature flags registered in `src/fleet/features.ts` specify `allowedEnvironments: ['dev']` or `['dev', 'prod']`. Default to `['dev']` for unreleased features. See [Shared Infrastructure & Cross-Domain Safety Rules](../../GEMINI.md#shared-infrastructure--cross-domain-safety-rules).
- **No ad-hoc `window.location` checks**: Do not create custom `window.location.hostname` checks or static boolean maps to toggle features. Always use `createFeatureFlag` and `useFeatureFlag` from `@/common/feature_flags`.


