---
name: prepare-cl
description: Verifies, commits, and uploads code changes as a Gerrit CL. Use when you have completed a task, fixed a bug, or are ready to submit changes for review.
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
     - **Commit Message Guidelines (3-Second Visual Review & 1-CL Fix Standard)**:
       - **Title**: Short, imperative summary (`<72` chars), e.g., `[fleet] Unify sentence-case status chips and raw filter search`.
       - **1. Visual Evidence First (`Screenshots:` & `Demo:` Immediately Below Title)**:
         Place the **Interactive Before/After Overlay Toggle (`http://go/zhangtiff-labs/before-after?before=<before_id>&after=<after_id>`)**, individual full-resolution `Before:` and `After:` screenshot links, and the **App Engine Live Demo (`luci-milo-dev.appspot.com`)** deep link at the **very top** of the commit message (before `TL;DR / Why:` or `Context:`).
         - **Multi-Page / Multi-Workflow Coverage**: Whenever a CL modifies visual elements across multiple pages or workflows (e.g., status chips across ChromeOS Devices, Repair Queue, Device Details, Android, and Browser), capture and list `Before` and `After` screenshots (plus `/before-after` toggle links) for **each major workflow that changed**:
         ```text
         [fleet] Unify sentence-case status chips and raw filter search

         Before / After (all pages):
         http://go/zhangtiff-labs/before-after?before=<b1>&after=<a1>&views=ChromeOS+Devices:<b1>:<a1>,Repair+Queue:<b2>:<a2>

         Screenshots (Before -> After):
         - ChromeOS Devices: https://screenshot.googleplex.com/<b1> -> https://screenshot.googleplex.com/<a1>
         - Repair Queue:     https://screenshot.googleplex.com/<b2> -> https://screenshot.googleplex.com/<a2>
         Demo: https://<short-version>-dot-luci-milo-dev.appspot.com/ui/fleet/p/chromeos/devices

         TL;DR / Why:
         Device tables across ChromeOS, Android, and Browser previously showed
         status badges in raw uppercase code strings (like NEEDS_REPAIR) with
         inconsistent backgrounds. This change updates all status chips and
         filter menus to use clean, readable labels ("Needs repair", "Ready")
         with action-based color coding (orange for automated recovery, red for
         manual repair), while still matching both readable and raw names in
         filter search.

         Context:
         - Formats status chips in DutStateCell, renderChipCell (Android and
           Browser), and ResourceStateChip using sentence-case labels and
           semantic tonal fills.
         - Updates StringListFilterCategory search and selection to match both
           display labels and raw enum values while emitting raw values in
           AIP-160 filter queries.

         Bug: b/444677406
         ```
       - **2. Plain-English `TL;DR / Why:` (No Code Jargon)**: Write 1-2 plain-English sentences describing the user-facing visual problem and benefit. Do **not** put `.tsx` filenames, CSS properties, or React hook names in `TL;DR / Why:`.
       - **3. Technical `Context:`**: Keep component names, file paths, and preserved workflow invariants (row height, URL filter compatibility) in a brief `Context:` section below `TL;DR / Why:`.
       - **Footer**: Include `Bug: b/XXXXXXX` (with the `b/` prefix). Always update modified first-party file headers to `// Copyright 2026 The LUCI Authors.`.
       - `Change-Id`: When committing a brand new branch for the first time (`-F`), **omit `Change-Id:` from the message file entirely** so Gerrit's `commit-msg` hook generates a unique SHA-1 hash. Only include `Change-Id:` when amending (`--amend`) an existing uploaded CL.

3. **Deploy App Engine UI Demo & Attach Before/After Screenshots (Mandatory for UI/Visual CLs)**:
   - **App Engine Live Demo vs. Static Easy Mock Rule**:
     - Whenever new or modified code is being added live to production in a CL, **always** use the **App Engine Live Demo (`luci-milo-dev.appspot.com`)** workflow ([deploy-ui-demo](../deploy-ui-demo/SKILL.md)).
     - Never substitute static Easy Mock links on a production Gerrit CL; Easy Mock prototypes are strictly for **longer-term ideas that are not yet at the CL stage**.
   - **3a. Deploy App Engine Live Demo (`luci-milo-dev`)**:
     - Follow [deploy-ui-demo](../deploy-ui-demo/SKILL.md) to deploy both `default` and `ui-new` services to `luci-milo-dev` under `--target-version=${MY_VERSION}`.
     - Construct a deep link pointing directly to the affected route, columns, and filters:
       `https://<short-version>-dot-luci-milo-dev.appspot.com/ui/fleet/p/chromeos/devices?c=id&c=dut_state`
   - **3b. Capture Individual Full-Resolution `Before` and `After` Screenshots & Build `/before-after` Toggle Link**:
     - Capture full-resolution `before.png` and `after.png` (`1440x1260`) for **each page or workflow where visual elements changed** (do not stitch squished side-by-side images).
     - **Uploading Screenshots (Googlers vs. Open-Source Contributors)**:
       - **Googlers**: Upload each `Before` and `After` PNG using the internal `http://go/screenshot` (`snipit`) CLI:
         ```bash
         /google/bin/releases/gemini-agents-snipit-cli/snipit            --file="milo/ui/.tmp/before.png"            --title="Before - ChromeOS Devices"
         /google/bin/releases/gemini-agents-snipit-cli/snipit            --file="milo/ui/.tmp/after.png"            --title="After - ChromeOS Devices"
         ```
         Then construct the 1-click full-resolution overlay toggle URL using `http://go/zhangtiff-labs/before-after?before=<before_id>&after=<after_id>` (and add `&views=Label1:b1:a1,Label2:b2:a2` when the CL spans multiple pages).
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
