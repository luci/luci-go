---
name: prepare-cl
description: >-
  Verifies, commits, and uploads Fleet Console code changes as a Gerrit CL with
  top-of-description App Engine Demo, http://go/ui-diff Before/After proof, and
  a concise 15-second Why/Context description. Use when completing a task,
  fixing a bug, or preparing a CL for review. Don't use for read-only code
  exploration or early static ideation mocks.
---

# Prepare CL Skill

> **Note**: This document contains instructions for AI code assistants working in this repository. Human developers can use it as a reference.

Use this skill when you have completed a task and need to prepare the changes for code review.

## Workflow

> [!IMPORTANT]
> **At the start of preparation**, you MUST copy the progress checklist below into your very next response to the user, and check off the steps sequentially as you complete them. This ensures structured progress visibility and prevents skipping verification steps.

Progress:
- [ ] Step 0: Branch Safety Check
- [ ] Step 1: Verification
- [ ] Step 2: Commit Changes
- [ ] Step 3: Deploy App Engine UI Demo & Attach Before/After Screenshots (Mandatory for UI/Visual CLs)
- [ ] Step 4: CL Upload

## Procedures

1. **Verification**:
   - Ensure you are in the `milo/ui` directory.
   - Run the linter to catch formatting and style issues:
     ```bash
     npm run lint
     ```
   - Run TypeScript type checking:
     ```bash
     npm run type-check
     ```
   - Run relevant Jest unit tests for the modified components/hooks:
     ```bash
     npm test -- <path_to_test_file>
     ```
   - *Note*: Do not proceed until all checks pass with zero errors and zero warnings.

2. **Commit Changes**:
   - Run `git status` to see modified files.
   - Stage changes: `git add <files>`.
   - Commit changes with a descriptive message.
     - **Commit Message Guidelines (15-Second Visual Review & `<= 100`-Word Standard)**:
       - **Title (`< 70` chars, ending with `.`)**: Imperative summary, e.g., `[fleet] Unify sentence-case status chips and raw filter search.`.
       - **1. Visual Evidence First (`Demo:`, `UI Diff:`, & `Screenshots:` Immediately Below Title)**:
         Place `Demo:`, `UI Diff: http://go/ui-diff?before=<before_id>&after=<after_id>&title=<Title>`, and full-resolution `Before:` / `After:` `http://screen/<id>` links at **line 1 immediately below the title** (before `Why:` and `Context:`, **never** buried at the bottom above `Bug:`).
         - **Zero Fake Mock UIs, Standalone HTML Cards, Preview Harnesses, or Crash Screens**: Every screenshot MUST be a full-app (`1600x1180`) capture of the **real, unedited App Engine Fleet Console UI shell** (top AppBar + populated table/page). Never render standalone HTML mockup cards, stitch side-by-side images (`sbs.png`), mount temporary in-app `"Preview Harness"` components, edit the DOM in DevTools, or upload a crashed React error screen as a `Before:` screenshot.
         ```text
         [fleet] Unify sentence-case status chips and raw filter search.

         Demo: https://c8548838-dot-luci-milo-dev.appspot.com/ui/fleet/p/chromeos/devices
         UI Diff: http://go/ui-diff?before=9s4K8TpryYNNLoQ&after=THfPi6ytvPKNqZU&title=Status+Chips
         Screenshots:
         - Before: http://screen/9s4K8TpryYNNLoQ
         - After:  http://screen/THfPi6ytvPKNqZU

         Why:
         - Device tables showed status badges in raw uppercase enums (like NEEDS_REPAIR) with inconsistent backgrounds (b/444677406).

         Context:
         - Format status chips in `dut_state_cell.tsx` and `resource_state_chip.tsx` using sentence-case labels and semantic tonal fills.
         - Update `string_list_filter_category.tsx` search to match both display labels and raw enum values.

         Bug: b/444677406
         ```
         - **Non-Visual `.tsx` Opt-Out Headers (Top of Description Only)**:
           - *Pattern 1 (Functional / Data-Fetching Demo with No Visual Changes)*:
             ```text
             Demo: https://c<CL>-dot-luci-milo-dev.appspot.com/ui/fleet/p/...
             Demo Notes: <what data or functional behavior to verify>
             UI Diff: N/A (no visual changes — <concrete reason>)
             ```
           - *Pattern 2 (Pure Non-Visual Opt-Out, e.g. telemetry or test IDs)*:
             ```text
             Demo: N/A (no user-visible UI changes — <concrete reason>)
             UI Diff: N/A (no user-visible UI changes — <concrete reason>)
             ```
       - **2. Plain-English `Why:` (`<= 35` words, 1-2 sentences/bullets, Zero Code Jargon)**: State the concrete user-facing problem and `b/<id>` without `.tsx` filenames, CSS properties, or React hook names (use `Why:`, not `TL;DR / Why:`, and omit editorial filler like *"Great news"* or *"Unfortunately"*).
       - **3. Concise `Context:` (`<= 65` words, `<= 2` main bullets + optional `- Spillover:` 3rd bullet)**: Summarize the technical change using **basename file references only** (`dut_state_cell.tsx`, never full `milo/ui/src/fleet/...` paths). Keep `Why:` + `Context:` `<= 100` words combined.
       - **4. Footers & Anti-Duplication**: Use `Bug: b/XXXXXXX` (with the `b/` slash prefix, **never** `Bug: b:XXXXXXX` colon syntax). Omit automated-test boilerplate (`Test: npm test`) unless manual or live verification was run. When running `git commit --amend`, replace the message cleanly—never concatenate a second commit message onto an existing one.
       - `Change-Id`: When committing a brand new branch for the first time (`-F`), **omit `Change-Id:` from the message file entirely** so Gerrit's `commit-msg` hook generates a unique SHA-1 hash. Only include `Change-Id:` when amending (`--amend`) an existing uploaded CL.

3. **Deploy App Engine UI Demo & Attach Before/After Screenshots (Mandatory for UI/Visual CLs)**:
   - **App Engine Live Demo vs. Static Easy Mock Rule**:
     - Whenever new or modified code is being added live to production in a CL, **always** use the **App Engine Live Demo (`luci-milo-dev.appspot.com`)** workflow ([deploy-ui-demo](../deploy-ui-demo/SKILL.md)).
     - Never substitute static Easy Mock links or fake HTML card mockups on a production Gerrit CL; Easy Mock prototypes are strictly for **longer-term ideas that are not yet at the CL stage**.
   - **3a. Deploy App Engine Live Demo (`luci-milo-dev`)**:
     - Follow [deploy-ui-demo](../deploy-ui-demo/SKILL.md) to deploy both `default` and `ui-new` services to `luci-milo-dev` under `--target-version=${MY_VERSION}`.
     - Construct a deep link pointing directly to the affected route, columns, and filters:
       `https://<short-version>-dot-luci-milo-dev.appspot.com/ui/fleet/p/chromeos/devices?c=id&c=dut_state`
   - **3b. Capture Individual Full-App `Before` and `After` Screenshots & Build `http://go/ui-diff` Link**:
     - Capture full-app (`1600x1180`) `before.png` (from `https://luci-milo-dev.appspot.com/ui/fleet/p/...`) and `after.png` (from `https://<short-version>-dot-luci-milo-dev.appspot.com/ui/fleet/p/...`) showing the real Fleet Console UI shell (never stitch side-by-side images or render isolated HTML cards).
     - **Uploading Screenshots (Googlers vs. Open-Source Contributors)**:
       - **Googlers**: Upload each `Before` and `After` PNG using `snipit` (`http://screen/<id>`) and construct the interactive diff link:
         `UI Diff: http://go/ui-diff?before=<before_id>&after=<after_id>&title=<Title>`
       - **External / Open-Source Contributors**: Attach the `Before` and `After` screenshots to the linked issue tracker (`b/...` or Monorail/GitHub issue) and paste the attachment URLs under `Screenshots:` at the top of the CL description.
   - *Note*: If App Engine deployment fails with `gcloud` auth errors in a headless environment, notify the developer with the exact one-line deploy command and still place the `Screenshots:` block at the top of the CL description.

4. **CL Upload**:
   - Run the upload non-interactively in Work In Progress (WIP) mode. For detailed flags and procedures on bypassing prompts and editor popups, refer to the [bypassing-interactive-prompts](../bypassing-interactive-prompts/SKILL.md) skill.
   - For stacked or dependent CLs, follow the sequential stack upload sequence in the [gerrit-workflows](../gerrit-workflows/SKILL.md) skill to prevent Gerrit from corrupting the relation chain.

   > [!CAUTION]
   > **Do NOT switch branches, stage/unstage files, or run other git operations while `git cl upload` is running in the background!**
   > `git cl upload` runs asynchronously and expects the repository state to remain stable. If you switch branches (e.g., checkout a downstream branch) before the upload has fully completed, the upload process will package and commit files from the *newly checked out* branch under the old CL, corrupting the Gerrit CL with unrelated changes. Always wait for the upload task to finish completely before doing any further git operations.

   - **Gerrit Verification**: After uploading, remote Gerrit Tryjobs/checks will run. NEVER mark a CL "ready" or ask for user submission consent until these remote Tryjobs/checks have fully passed. Polling remote checks can take time (5-15+ minutes); set a timer using the `schedule` tool to go idle and check back.
   - **Transition to Review**: Do not mark the CL as "Ready" or send review emails. Leave it in the WIP state for the user to explicitly review and transition when ready.

5. **Advanced Git Operations for Agents**:

   ### Handling Multiple CLs (Standalone vs. Stacked)
   - **Rule**: Stacks of CLs should ONLY be used for changes that are **actually dependent on each other** (e.g., Stage 2 depends on modifications or components introduced in Stage 1).
   - **Rule**: If changes are independent (even if developed within the same coding session), they **MUST** be submitted as separate CLs tracking `origin/main` directly. Do not stack them just because they are part of the same session or task.
   - **Workflow for Standalone CLs**:
     1. Create a new branch from `origin/main` for the independent changes: `git checkout -b <branch-name> origin/main`.
     2. Stage and commit only the files belonging to this change.
     3. Upload using `git cl upload`.

   ### Avoiding Dirty Tree Traps
   Do not use `git add -A` or `git add .` blindly. If you created temporary directories or output files, they will pollute your branch. Follow the staging safety workflows in the [preventing-workspace-leakage](../preventing-workspace-leakage/SKILL.md) skill to isolate generated/transient files inside `.tmp/`.

   ### Splitting a Single Commit
   If you accidentally combined unrelated changes into a single commit and want to split it:
   - **Workflow**:
     1. Undo the last commit but keep modifications: `git reset --mixed HEAD~1`.
     2. Stage a subset of changes: `git add <specific_files>`.
     3. Commit the subset: `git commit -m "Part 1..."`.
     4. Repeat for remaining changes.

   ### Building on Other Ongoing Reviews
   If you need to build on top of another developer's in-flight CL:
   - **Workflow**:
     1. Create a new branch: `git checkout -b dependent_branch`.
     2. Pull the CL: `git cl patch -f <issue_number>`.
     3. Apply your changes on top.

6. **Rebasing and Handling Merge Conflicts**:
   - Ensure there are no merge conflicts with the upstream branch before considering a CL done.
   - Follow the rebase and conflict resolution guidelines in the [gerrit-workflows](../gerrit-workflows/SKILL.md) skill to perform clean rebases on standalone or stacked branches without corrupting history.

## Guardrails

> [!IMPORTANT]
> - **No Bypass Tags**: Do not include bypass directives (like `--bypass-hooks` or `--bypass-watchdog`) unless explicitly authorized by the user or required due to pre-existing upstream failures at HEAD.
> - **Anti-Flip-Flopping & Iteration Cap**: If you are fixing lints or test failures, keep track of your history. If your fix reverts a previous commit or changes the same lines back and forth, stop. Limit autonomous repair loops to a maximum of 3 iterations before asking the user for help.
