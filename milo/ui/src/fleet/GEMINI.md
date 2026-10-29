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

### 2. Definition of Done, 15-Second Commit Description & Real-UI Visual Proof
A task or frontend CL (including autonomous 1-CL fixes) is complete when:
- **Self-Review**: You run [senior-reviewer](./.agents/skills/senior-reviewer/SKILL.md) and resolve all findings (plus any Gerrit `ChromiumCodeReviewAgent` findings after upload).
- **UX & PM Review**: For visual or flow changes, you run [ux-pm-review](./.agents/skills/ux-pm-review/SKILL.md) to check PM alignment, Orwell writing rules, and UX principles.
- **Verification**: Tests, lints (`npm run lint`), and type checks (`npm run type-check`) pass cleanly with 0 errors and 0 warnings.
- **Visual Proof Front-and-Center (`Demo:`, `UI Diff:`, & `Before:`/`After:` First)**:
  - For any visual UI change, place `Demo: https://<version>-dot-luci-milo-dev.appspot.com/ui/fleet/p/...` (via [deploy-ui-demo](./.agents/skills/deploy-ui-demo/SKILL.md)), `UI Diff: http://go/ui-diff?before=<before_id>&after=<after_id>&title=<Title>`, and full-resolution `Before: http://screen/<before_id>` / `After: http://screen/<after_id>` links **at the very top of the CL description (line 1 immediately below the title, never buried at the bottom near `Bug:`)**.
  - **Zero Fake Mock UIs, Standalone HTML Cards, Preview Harnesses, or Crash Screens**: Capture full-app (`1600x1180`) screenshots of the **real, unedited App Engine UI shell** (top AppBar + populated page table). Never render standalone HTML mockup cards, stitch side-by-side images (`sbs.png`), mount temporary in-app `"Preview Harness"` components, edit the DOM in DevTools, or upload a crashed React error screen as a `Before:` screenshot.
  - **Non-Visual `.tsx` Opt-Outs (Top of Description Only)**: If a `.tsx` CL has zero visual changes, place either (1) `Demo: <url>`, `Demo Notes: <what to verify>`, and `UI Diff: N/A (no visual changes — <reason>)` (for data/RPC changes), or (2) `Demo: N/A (no user-visible UI changes — <reason>)` and `UI Diff: N/A (no user-visible UI changes — <reason>)` (for pure telemetry/test-ID plumbing) at the top of the description.
- **15-Second `Why:` (`<= 35` words) & `Context:` (`<= 65` words, `<= 3` bullets)**:
  - Write a 1-2 bullet/sentence plain-English `Why:` (`<= 35` words) stating the user-facing problem and `b/<id>` with zero file paths or code jargon (use `Why:`, not `TL;DR / Why:`).
  - Write a concise `Context:` (`<= 65` words, `<= 2` main bullets + optional `- Spillover:` 3rd bullet) naming the technical fix using **basename file references only** (`devices_table.tsx`, never full repo paths). Keep `Why:` + `Context:` `<= 100` words combined (see [prepare-cl](./.agents/skills/prepare-cl/SKILL.md)).
- **Direct Upload**: Upload the CL to Gerrit in WIP mode via `git cl upload` (never concatenate duplicate commit messages or use `Bug: b:<id>` colon syntax; always use `Bug: b/<id>`).

### 2b. When to Use an App Engine Live Demo (`luci-milo-dev`) vs. a Static Easy Mock Prototype

| Workflow Path | When to Use | Deliverable & Verification Package |
| :--- | :--- | :--- |
| **App Engine Live Demo (`luci-milo-dev.appspot.com`) + `http://go/ui-diff` Before/After Proof** *(Required whenever code is added to prod)* | All Gerrit CLs adding or updating live production UI code—including small UX fixes, visual polish/beautification, chip/typography formatting, new columns/filters, bug fixes, and autonomous 1-CL fixes (`<= 150` prod LOC). | Standard Gerrit CL on `luci-go` with: (1) `Demo: https://<short-version>-dot-luci-milo-dev.appspot.com/ui/fleet/p/...`, `UI Diff: http://go/ui-diff?before=<id>&after=<id>&title=<Title>`, and full-app `Before:`/`After:` `http://screen/<id>` links at the very top of the description, and (2) `<= 100`-word `Why:` + `Context:`. Never substitute a static Easy Mock link or fake HTML card mockup for an App Engine demo on a production CL. |
| **Static Easy Mock Prototype ([ux-prototyping](./.agents/skills/ux-prototyping/SKILL.md))** *(Strictly for longer-term ideas not yet at the CL stage)* | Longer-term product ideas that are **not yet at the CL stage**: early PM/UX ideation, multi-option [design-tournament](./.agents/skills/design-tournament/SKILL.md) explorations, or net-new multi-page concepts requiring mocked backend RPCs that do not exist yet. | Self-contained static mock bundle with client-side mock pRPC fixtures (`settings.js`) for early stakeholder exploration before writing production CLs. |

### 3. Self-Review & UX/PM Audits
- Run [senior-reviewer](./.agents/skills/senior-reviewer/SKILL.md) to review diffs before uploading CLs.
- For major UI features, run [design-tournament](./.agents/skills/design-tournament/SKILL.md) to compare layout options.
- Audit copy and layouts against [writing-and-ux-principles](./.agents/skills/writing-and-ux-principles/SKILL.md) (Orwell writing rules, Material writing spec, and Laws of UX).

### 4. Coding Conventions
- Rely on TypeScript type inference and narrowing instead of type casting (`as Type`).
- **Legacy Naming Conventions (`_OLD`)**: Do not copy or extend files or exports with `_OLD` / `_deprecated` naming suffixes.
- **Reuse Shared Components & Utilities**: Respect the domain folder organization in `src/fleet/components/` and `src/fleet/utils/`. Before creating new UI widgets, dialogs, or formatting helpers, scan `src/fleet/components/` and `src/fleet/utils/` for existing implementations to reuse or extend (see [components/README.md](./components/README.md)).

### 4b. Styling Conventions (read before writing any `sx`, `css`, or `styled`)
The canonical guide is [theme/README.md](./theme/README.md). In short:
- **Never write raw style literals** (hex colors, `borderRadius`, `boxShadow`, `fontSize`) in a component. Use tokens from `@/fleet/theme/tokens`, recipes from `@/fleet/theme/recipes`, or theme variants.
- **Use the shared pieces**: `<Button variant="toolbar">` for toolbar/header actions, `<Chip variant="filter">` for filters, `<StatusPill>` / `<TaskResultPill>` for any colored state, `surfaceCardSx` / `MetricsContainer` for cards, `metricTileSx` / `subMetricRowSx` for interactive KPI tiles and sub-metrics, `pageHeaderSx` + `dockedTabStripSx` for detail pages.
- **Three radii only**: `radii.sm` (6px, including multi-line KPI tiles and text buttons), `radii.md` (12px for cards/tables), `radii.pill` (9999px for single-line pills/chips). **One surface level**: no bordered card inside a bordered card.
- **Accessibility (a11y)**: Never set `outline: 'none'` on `:focus-visible` (use `focusRingSx` / `metricTileSx` / `subMetricRowSx`), always include `aria-label` on icon-only `<IconButton>`s, and preserve WCAG AA (`>= 4.5:1`) contrast without `opacity` dimming.
- **Call-site `sx` is for layout only** (margin, padding, gap, flex). A missing look means a missing token or recipe: add it to `src/fleet/theme/`, then use it.
- `src/fleet/theme/style_conventions.test.ts` fails if raw literals in `src/fleet` grow. Do not raise its baseline; lower it when you remove literals.

### 4c. Frontend Interaction, Accessibility & Review Invariants
- **Cursor Semantics vs. Click Affordance**: Use `cursor: 'help'` (or `'default'`) on tooltip-only triggers, read-only chips, and progress bars that have no click action. Reserve `cursor: 'pointer'` strictly for clickable controls.
- **Keyboard Accessibility (`a11y`) for Tooltips & Icon-Only Controls**: Always add `tabIndex={0}` to non-button `<Tooltip>` trigger children (`<Box>`, `<Typography>`, `<span>`) so keyboard users can open tooltips. Always provide a descriptive `aria-label` on icon-only links (`<a>`) and buttons (`<IconButton>`).
- **Click Event Bubbling in Selectable Rows**: Never call `e.stopPropagation()` on passive/unclickable child badges or chips inside clickable `<ListItem>` or table rows, as it swallows clicks and prevents row selection.
- **Direct Module Imports Over `.tsx` Re-Exports**: Never re-export utility functions or formatting constants from `.tsx` React component files. Import utilities and constants directly from `src/fleet/utils/` or `src/fleet/constants/`.
- **`ts-proto` `int64` String Defaults**: `ts-proto` deserializes `int64` fields as strings where the default/missing value is `'0'` (not `''`). Check `val && val !== '0'` and use `'0'` in mock fixtures.
- **No Redundant `pageSize: 1` Summary RPCs**: Never fire a separate `pageSize: 1` `List*` RPC solely to read a total or summary count that is already returned by the primary table query on the same page.
- **Deterministic Test Mock Cleanup**: Always restore mocks (`jest.restoreAllMocks()` / `mockRestore()`) inside `afterEach()` hooks rather than at the end of `it()` / `test()` bodies so assertion failures never leak mocks into subsequent tests.

### 4d. Testing Strategy & Anti-Brittleness Rules
- **Frontend Test Hierarchy**:
  - **Component & Page Integration Tests (`*.test.tsx`)**: Complex stateful widgets (e.g., `FilterBar`, `FCDataTable`, filter dropdowns) and page components (`*Page.test.tsx`) serve as the frontend front-door integration layer. Mount real child component trees and mock **only the network boundary** (`FleetConsoleClient` pRPC methods).
  - **Pure Logic Unit Tests (`*.test.ts`)**: Write unit tests for pure, self-contained logic (AIP-160 filter parsers, filter adapters, formatters, math/SLA helpers, validators) and non-integration-testable edge cases.
- **Anti-Brittleness Rules**:
  - **No raw style / CSS layout unit tests**: Do not add unit tests asserting 1:1 on raw `sx` style objects (`expect(STYLE_OBJ.padding).toBe(...)`) or JSDOM layout properties (`toHaveStyle({ minWidth: ..., padding: ... })`). Visual consistency is enforced by `style_conventions.test.ts`, ESLint, and real App Engine screenshots (`go/ui-diff`).
  - **No redundant thin-wrapper unit tests**: Do not add separate unit tests for thin presentational wrappers (`FilterChip`, individual table cells/rows) that have no independent business logic and are already exercised through parent integration tests.
  - **No hardcoded domain/config lists**: Iterate over exported constants/enums dynamically rather than duplicating literal values in tests.
  - **No internal hook mocking**: Do not mock internal helper hooks (`jest.mock('./use_...')`) when testing components; test through the user-facing interface (`getByRole`, `getByLabelText`, `findByText`, `userEvent`).

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
- **Feature flags (`createFleetFeatureFlag` & ESLint)**: Register flags in `src/fleet/features.ts` via `createFleetFeatureFlag` (which requires `allowedEnvironments: ['dev']` or `['dev', 'prod']` at compile time); direct `createFeatureFlag` imports and `window.location.hostname` checks in `src/fleet/` are blocked by `eslint.config.js`.



