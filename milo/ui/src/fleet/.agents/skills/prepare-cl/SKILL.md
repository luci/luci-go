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
- [ ] Step 3: Upload UI Demo (Optional/Conditional)
- [ ] Step 4: Optional Upload

## Procedures

0. **Branch Safety Check**:
   - **Sync Before Branching**: Before creating a new task branch, you MUST run `git fetch origin` to update your local repository refs. Always create the branch off the updated remote main: `git checkout -b <branch-name> origin/main`. Creating branches from stale local representations of `origin/main` is a major cause of early merge conflicts on Gerrit.
   - **New Task Branching**: Generally start a new branch when starting a new task.
   - **Check Active CL Tracking**: When resuming or editing an existing CL, you MUST run `git cl status` or `git branch -vv` to verify that the active local branch is tracking the correct Gerrit CL ID.
   - **Check for Unrelated Changes**: Double check that the branch you're currently on doesn't have unrelated changes before you upload a CL. Run `git log origin/main..HEAD` to see the commits on your branch relative to main, and ensure they are all related to the current task.

1. **Verification**:
   - Format modified files: `git cl format`.
   - Run the [project-verification](../project-verification/SKILL.md) skill (lint, test, type-check) to ensure no regressions. Alternatively, run targeted checks:
     - Lint changed files: `npm run lint -- <changed_file_paths>`
     - Run unit tests: `npm run test -- <test_file_paths>`
     - Run type checks: `npm run type-check`
   - Run local presubmits to check licenses, syntax, and static analysis: `git cl presubmit`.
   - Run the [senior-reviewer](../senior-reviewer/SKILL.md) skill to get a self-review and address feedback.

2. **Commit Changes**:
   - Run `git status` to see modified files.
   - Stage changes: `git add <files>`.
   - Commit changes with a descriptive message.
     - **Commit Message Guidelines**:
       - Title: Short, descriptive summary.
       - Body: Explain what changed and why. Include business or design context (e.g., "This is needed to support the new Android health layout..."). If it fixes a bug, explain the fix.
       - Footer: If the change fixes a bug, include reference in the format `Bug: b/XXXXXXX` (note the `b/` prefix).
     - Example: `git commit -m "[fleet console] Fix CSV export missing columns by preserving URL columns\n\n... details ...\n\nBug: b/515102813"`

3. **Upload UI Demo**:
   - For UI changes, you **MUST** upload a demo to the App Engine development environment and include the demo link in the CL description for reviewer verification, unless deployment is blocked by infrastructure/authentication issues.
   - **Unified App Engine Deployment (Required)**:
     - Do NOT deploy only the individual `ui-new` service (e.g., via `make deploy-ui-demo`). App Engine handles request routing via a central dispatcher service (`ui-dispatcher`). If you only deploy `ui-new` without updating `ui-dispatcher` under the same version ID, requests to static assets (like `/ui/immutable/...`) will fail with 404 console errors, resulting in a blank screen.
     - Instead, always build the UI (`npm run build` with standard configurations) and upload **all services together** using a custom target version.
     - Navigate to the `go.chromium.org/luci/milo` project root (one level up from the `ui` directory) and run:
       `PATH=$PATH:~/depot_tools ../../../../env.py gae.py upload -p ./ -A luci-milo-dev -f --target-version=<short-name>`
   - **Target Version SSL Length Limits**:
     - App Engine has a strict 63-character length limit on subdomains for SSL certificates. If the version name (automatically generated from branch name, user, etc.) exceeds this limit, the SSL certificate negotiation will fail, resulting in a broken HTTPS link.
     - You **MUST** override the target version with a short, clean, and unique name (e.g., `bh-summary` or `fc-export-csv`) using the `--target-version=<short-name>` flag as shown above.
   - **Accessing the Demo**:
     - Once deployed, the staging instance is accessible at the standard unified URL structure:
       `https://<short-name>-dot-luci-milo-dev.appspot.com/ui/fleet/p/chromium/devices`
     - Place this demo link at the top of the CL description (immediately after the title line). Make sure it points directly to the affected page(s).
   - *Note*: If deployment fails with auth errors (e.g., `Login first using 'gcloud auth login'`), notify the developer and ask them to run it on your behalf. Do not block the task on this failure.

4. **Optional Upload (WIP Workflows)**:
   - Read the existing CL description/metadata first if amending. Ensure you preserve existing metadata tags (like `Bug:`, `Reviewers:`, `Cc:`) in the description so you don't overwrite user configurations.
   - Attempt to run `git cl upload`.
   - **WIP Mode for New CLs Only**: Upload new CLs (initial upload) in Work In Progress (WIP) mode to suppress premature notifications to reviewers and watchlisted groups.
   - **Maintain Review State for Existing CLs**: For existing CLs that are already under active review, do **NOT** pass WIP flags (like `-o wip` or `--wip`) during subsequent uploads. Pushing an active review CL back to WIP is highly disruptive because it hides the CL from reviewers' active dashboards.
   - *Note*: If this fails with an error like `chmod ~/.sso: operation not permitted` or a path-specific permission error, it is due to sandbox restrictions preventing the agent from modifying home directory files. In this case, do not block the task; notify the developer and ask them to run `git cl upload` on your behalf. (Alternatively, the developer may choose to disable the sandbox to allow direct uploads).
   - **Updating CL Description Non-Interactively**: If you amend the commit message locally but no new code changes are detected, running `git cl upload` will not update the description on Gerrit. In this case, or to update the description directly and bypass editor blocks, write the new description to a temporary file (e.g. `.tmp/desc.txt`) and run:
     `git cl description -n - < .tmp/desc.txt`
   - **Gerrit Verification**: After uploading, remote Gerrit Tryjobs/checks will run. NEVER mark a CL "ready" or ask for user submission consent until these remote Tryjobs/checks have fully passed. Polling remote checks can take time (5-15+ minutes); set a timer using the `schedule` tool to go idle and check back.
   > When uploading from background tasks, you can bypass all interactive prompts and text editor popups using these official non-interactive options:
   > - **For New CLs (Initial Upload)**: **Always use the `--wip` flag** along with `-f` and `--commit-description=+` to ensure the CL starts as WIP:
   >   `git cl upload -f --wip --commit-description=+`
   > - **For Existing CLs (Subsequent Updates)**:
   >   - If the CL is currently in WIP state: Use `-t` or `-T` to specify the patchset title and keep it in WIP:
   >     `git cl upload -t "My patchset title"`
   >   - If the CL is already in Active Review: Run the upload command without any WIP options to preserve its active status:
   >     `git cl upload -t "Address review feedback"`
   - **Transition to Review**: For new CLs, do not mark the CL as "Ready" or send review emails. Leave it in the WIP state for the user to explicitly review and transition when ready.
   >
   > (If you are encountering other interactive prompts, bypass them using the non-interactive option: `EDITOR=touch git cl upload -f --commit-description=+`)
   >
   > > [!CAUTION]
   > > **Do NOT switch branches, stage/unstage files, or run other git operations while `git cl upload` is running in the background!**
   > > `git cl upload` runs asynchronously and expects the repository state to remain stable. If you switch branches (e.g., checkout a downstream branch) before the upload has fully completed, the upload process will package and commit files from the *newly checked out* branch under the old CL, corrupting the Gerrit CL with unrelated changes. Always wait for the upload task to finish completely before doing any further git operations.

5. **Advanced Git Operations for Agents**:

   ### Handling Multiple CLs (Standalone vs. Stacked)
   - **Rule**: Stacks of CLs should ONLY be used for changes that are **actually dependent on each other** (e.g., Stage 2 depends on modifications or components introduced in Stage 1).
   - **Rule**: If changes are independent (even if developed within the same coding session), they **MUST** be submitted as separate CLs tracking `origin/main` directly. Do not stack them just because they are part of the same session or task.
   - **Workflow for Standalone CLs**:
     1. Create a new branch from `origin/main` for the independent changes: `git checkout -b <branch-name> origin/main`.
     2. Stage and commit only the files belonging to this change.
     3. Upload using `git cl upload`.

   ### Cleaning Up Accumulating Commits (Unstacking & Decoupling Branches)
   When rebasing local branches that were previously stacked, a branch may carry over commits and file modifications from its old parent branches, causing `git cl upload` to pull in unrelated changes.
   To surgically clean up a branch so it contains exactly one commit with only the desired changes:
   1. Checkout `origin/main` to a temporary branch:
      `git checkout -b temp-clean-branch origin/main`
   2. Checkout only the files belonging to the specific CL from your dirty local branch:
      `git checkout <dirty-branch-name> -- path/to/file1.tsx path/to/file2.ts`
   3. Commit the clean files:
      `git commit -m "[Category] Commit description message"`
   4. Switch back to your dirty branch and hard reset it to the clean state:
      `git checkout <dirty-branch-name> && git reset --hard temp-clean-branch`
   5. Delete the temporary branch:
      `git branch -D temp-clean-branch`
   This guarantees the branch contains exactly one commit on top of `origin/main` with no unrelated tracking files.

   ### Creating Stacked/Chained CLs with Gerrit Dependencies
   - **Rule**: Use stacked chains ONLY when changes are sequentially dependent.
   - **Workflow**:
     1. **Start from the base branch/CL**: Ensure you are on the branch of the first CL (e.g., `branch-1` for `CL 1`).
     2. **Create a dependent branch**: Run `git new-branch --upstream-current <new-branch-name>` or create a branch and configure its tracking manually: `git branch --set-upstream-to=branch-1`.
     3. **Apply and commit changes**: Apply your changes and commit them to `branch-2`.
     4. **Upload to Gerrit**: Run `git cl upload -f --wip --commit-description=+`. Because `branch-2` was created with `--upstream-current` or tracks `branch-1` as upstream, Gerrit will recognize the dependency structure, and the new CL will show as depending on the parent CL in the Gerrit UI.
     5. **Uploading subsequent edits**:
        - To update the base CL (`CL 1`), switch back to `branch-1`, make changes, commit (amending if desired: `git commit --amend`), and upload.
        - After updating `branch-1`, you must rebase `branch-2` onto the updated `branch-1`. Use `git rebase-update` to automatically and cleanly rebase all local stacked branches on their respective upstreams.

   ### Updating CL Description Non-Interactively
   Agents cannot use interactive editors (like `vim`) that `git cl upload` or `git cl description` open by default.
   - **Workflow (Amended Commit Message)**:
     If you amended a local commit message and want to push it directly as the Gerrit CL description:
     1. Run `git cl description -n +`.
     2. **Crucial Rule**: The local commit *must* contain the exact `Change-Id` footer matching the target Gerrit CL issue (check association via `git cl status`). If they mismatch, Gerrit will reject the update with a `409 Conflict (wrong Change-Id footer)` error.
   - **Workflow (Explicit Text/File)**:
     Alternatively, to specify a description explicitly:
     1. Write the new description to a temporary file: `.tmp/desc.txt`.
     2. Set the description using standard input: `git cl description -n - < .tmp/desc.txt`.

   ### Avoiding Dirty Tree Traps
   Do not use `git add -A` or `git add .` blindly. If you created temporary directories or output files, they will pollute your branch unless they are channeled into the gitignored `.tmp/` folder (which is ignored at the `milo/ui` root). See [preventing-workspace-leakage](../preventing-workspace-leakage/SKILL.md) for detailed staging safety workflows.
   - **Rule 1**: Always write transient or generated files (e.g. temporary logs, test results, or generated queries) to the local gitignored `.tmp/` directory (e.g., `milo/ui/.tmp/`).
   - **Rule 2**: Always check `git status` before staging, and prefer staging files explicitly by name instead of using catch-all commands.

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

   ### Conflict Resolution Helpers
   To make resolving merge conflicts easier, you can enable these repository-local git settings (never modify global configs without explicit permission):
   - **Diff3 Style**: `git config --local merge.conflictstyle diff3` (Shows base version in conflict markers).
   - **Git Rerere**: `git config --local rerere.enabled true` (Remembers how you resolved conflicts before).

6. **Rebasing and Handling Merge Conflicts**:
   - Ensure there are no merge conflicts with the upstream branch before considering a CL done.
   - **Safety Check**: Run `git status` first to ensure a clean working directory before attempting a rebase.
   - Run `git pull --rebase origin main` (or the configured upstream branch) to sync with upstream.
   - **Avoiding Conflicts**:
     - **Keep CLs Small**: Focus on a single task per CL to reduce the chance of overlapping edits.
     - **Rebase Frequently**: Sync with upstream often to catch and resolve conflicts early.
     - **Coordinate Edits**: If you plan to make sweeping changes or edit a highly active file, coordinate with other developers by asking the user (your pair programmer) to coordinate on your behalf, or by checking for active CLs touching the same files.
   - **Handling Conflicts**:
     - If conflicts occur, do not simply remove conflict markers.
     - Analyze both versions of the conflicting code blocks (`<<<<<<<` and `>>>>>>>`).
     - Choose the logically correct implementation or combine them if needed.
     - Ensure syntax validity after resolution.
     - Run a lint check (`npm run lint`) on the file before staging!
     - After resolving, stage the files: `git add <files>`.
     - Continue the rebase: `git rebase --continue`.
     - Re-run tests after rebasing to ensure no regressions.

## Guardrails

> [!IMPORTANT]
> - **No Bypass Tags**: Do not include bypass directives (like `--bypass-hooks` or `--bypass-watchdog`) unless explicitly authorized by the user or required due to pre-existing upstream failures at HEAD.
> - **Anti-Flip-Flopping & Iteration Cap**: If you are fixing lints or test failures, keep track of your history. If your fix reverts a previous commit or changes the same lines back and forth, stop. Limit autonomous repair loops to a maximum of 3 iterations before asking the user for help.
