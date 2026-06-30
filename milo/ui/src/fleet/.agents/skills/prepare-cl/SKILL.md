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
- [ ] Step 4: CL Upload

## Procedures

0. **Branch Safety Check**:
   - **Sync Before Branching**: Before creating a new task branch, you MUST run `git fetch origin` to update your local repository refs. Always create the branch off the updated remote main: `git checkout -b <branch-name> origin/main`. Creating branches from stale local representations of `origin/main` is a major cause of early merge conflicts on Gerrit.
   - **New Task Branching**: Generally start a new branch when starting a new task.
   - **Check Active CL Tracking**: When resuming or editing an existing CL, you MUST run `git cl status` or `git branch -vv` to verify that the active local branch is tracking the correct Gerrit CL ID.
   - **Check for Unrelated Changes**: Double check that the branch you're currently on doesn't have unrelated changes before you upload a CL. Run `git log origin/main..HEAD` to see the commits on your branch relative to main, and ensure they are all related to the current task.

1. **Verification**:
   - Format modified source files: `git cl format`.
   - Strip trailing whitespaces from modified Markdown/text files. Run this Python command on changed files:
     ```bash
     python3 -c "
     import sys
     for x in sys.argv[1:]:
         with open(x, 'r') as f:
             lines = f.readlines()
         with open(x, 'w') as f:
             f.writelines(l.rstrip() + '\n' for l in lines)
     " <modified_files>
     ```
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
