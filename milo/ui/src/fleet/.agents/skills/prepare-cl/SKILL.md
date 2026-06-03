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
   - **New Task Branching**: Generally start a new branch when starting a new task.
   - **Check for Unrelated Changes**: Double check that the branch you're currently on doesn't have unrelated changes before you upload a CL. Run `git log origin/main..HEAD` to see the commits on your branch relative to main, and ensure they are all related to the current task.

1. **Verification**:
   - Run the [project-verification](../project-verification/SKILL.md) skill (lint, test, type-check) to ensure no regressions.
   - Run the [senior-reviewer](../senior-reviewer/SKILL.md) skill to get a self-review and address feedback.

2. **Commit Changes**:
   - Run `git status` to see modified files.
   - Stage changes: `git add <files>`.
   - Commit changes with a descriptive message.
     - **Commit Message Guidelines**:
       - Title: Short, descriptive summary.
       - Body: Explain what changed and why. Include business or design context (e.g., "This is needed to support the new Android health layout..."). If it fixes a bug, explain the fix.
       - Footer: If the change fixes a bug, include reference in the format `Bug: b/XXXXXXX` (note the `b/` prefix).
     - Example: `git commit -m "[fleet] Fix CSV export missing columns by preserving URL columns\n\n... details ...\n\nBug: b/515102813"`

3. **Upload UI Demo**:
   - For UI changes, consider uploading a demo to a dev environment.
   - **Demo Link Requirement**: You **MUST** include a demo link in the CL for non-trivial frontend changes with user facing impact, unless deployment fails and cannot be resolved immediately.
   - Run `make deploy-ui-demo` in the UI directory.
   - *Note*: If this fails with auth errors (e.g., `Login first using 'gcloud auth login'`), notify the developer and ask them to run it on your behalf. Do not block the task on this failure. (Alternatively, the developer may choose to disable the sandbox to allow direct uploads).
   - If successful, include the demo link in the CL description (typically printed in the output of the deploy command). **The demo link should ALWAYS be at the top of the commit message (right after the title line).** **Make sure the demo link goes directly to the affected pages for convenience.**
   - Include testing instructions for the reviewer in the CL description, encouraging them to also test edge cases or related functionality that might break.

4. **Optional Upload**:
   - Attempt to run `git cl upload`.
   - *Note*: If this fails with an error like `chmod ~/.sso: operation not permitted` or a path-specific permission error, it is due to sandbox restrictions preventing the agent from modifying home directory files. In this case, do not block the task; notify the developer and ask them to run `git cl upload` on your behalf. (Alternatively, the developer may choose to disable the sandbox to allow direct uploads).
   > When uploading from background tasks, you can bypass all interactive prompts and text editor popups using these official non-interactive options:
   > - **For New CLs (Initial Upload)**: Use `-f` to force yes to prompts and `--commit-description=+` to load the description directly from the HEAD commit message instead of opening an editor:
   >   `git cl upload -f --commit-description=+`
   > - **For Existing CLs (Subsequent Updates)**: Use `-t` to specify the patchset title directly from the command line:
   >   `git cl upload -t "My patchset title"`
   >   *(Alternatively, use `-T` or `--skip-title` to automatically use the last commit message as the title)*
   >
   > (If you are encountering other interactive prompts, bypass them using the non-interactive option: `EDITOR=touch git cl upload -f --commit-description=+`)
   >
   > > [!CAUTION]
   > > **Do NOT switch branches, stage/unstage files, or run other git operations while `git cl upload` is running in the background!**
   > > `git cl upload` runs asynchronously and expects the repository state to remain stable. If you switch branches (e.g., checkout a downstream branch) before the upload has fully completed, the upload process will package and commit files from the *newly checked out* branch under the old CL, corrupting the Gerrit CL with unrelated changes. Always wait for the upload task to finish completely before doing any further git operations.

5. **Advanced Git Operations for Agents**:

   ### Handling Multiple CLs
   If a task involves independent changes (e.g., tests vs documentation, or core logic vs skill updates), consider splitting them into separate CLs to make review easier and safer.
   - **Workflow**:
     1. Create a new branch from `origin/main` for the independent changes.
     2. Cherry-pick or manually apply the relevant changes to that branch.
     3. Upload as a new CL using `git cl upload`.
     4. Ensure the branches do not depend on each other unless strictly necessary.

   ### Creating Stacked/Chained CLs with Gerrit Dependencies
   When you want to split a larger task into a series of dependent CLs (stacked changes):
   - **Workflow**:
     1. **Start from the base branch/CL**: Ensure you are on the branch of the first CL (e.g., `branch-1` for `CL 1`).
     2. **Create a dependent branch**: Run `git new-branch --upstream-current <new-branch-name>`. This creates a new branch (e.g., `branch-2`) tracking `branch-1` as its upstream.
     3. **Apply and commit changes**: Apply your changes and commit them to `branch-2`.
     4. **Upload to Gerrit**: Run `EDITOR=touch git cl upload -f --commit-description=+`. Because `branch-2` was created with `--upstream-current`, Gerrit will recognize the dependency structure, and the new CL will show as depending on the parent CL in the Gerrit UI.
     5. **Uploading subsequent edits**:
        - To update the base CL (`CL 1`), switch back to `branch-1`, make changes, commit (amending if desired: `git commit --amend`), and upload.
        - After updating `branch-1`, you must rebase `branch-2` onto the updated `branch-1`. Since `branch-1` was amended or rebased, a simple `git rebase branch-1` can cause git to incorrectly re-apply old commits, triggering merge conflicts. Instead, use one of these methods:
           - **Recommended**: Run `git rebase-update` from any branch. This depot_tools command automatically and cleanly rebases all local stacked branches on their respective upstreams.
           - **Manual Git**: Switch to `branch-2` and run `git rebase --onto branch-1 <old-branch-1-commit-hash> branch-2` (where `<old-branch-1-commit-hash>` is the commit hash of `branch-1` before it was amended/rebased).

   ### Updating CL Description Non-Interactively
   Agents cannot use interactive editors (like `vim`) that `git cl upload` or `git cl description` might open.
   - **Workflow**:
     1. Write the new description to a temporary file: `desc.txt`.
     2. Set the description using standard input: `git cl description -n - < desc.txt`.
     3. Remove the temporary file: `rm desc.txt`.

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
