---
name: managing-parallel-workspaces
description: Creates and manages isolated git worktrees for concurrent task execution. Use this skill before starting any code modification task that runs concurrently with other agent tasks in the same workspace.
---

# Managing Parallel Workspaces

## Quick Start

To safely isolate parallel tasks using Git Worktrees:

1. **Create Worktree**:
   Create the worktree directory strictly inside the `milo/ui/` project directory (e.g., as a subdirectory under `milo/ui/.worktrees/`) so it is ignored by the project-level `.gitignore` rules.
   - If executing from the `milo/ui/` directory:
     ```bash
     git worktree add -b <new_branch_name> .worktrees/<new_branch_name> origin/main
     ```
   - If executing from the `luci` repository root (e.g., `infra/go/src/go.chromium.org/luci/`):
     ```bash
     git worktree add -b <new_branch_name> milo/ui/.worktrees/<new_branch_name> origin/main
     ```

2. **Move Context**: Switch your agent's execution Cwd to the `<worktree_path>` directory for all edits and tests.
   *Tip:* Creating a new worktree checkouts a clean directory structure, but it will not contain the `node_modules` dependencies required to run lints and type-checks. To avoid executing a slow `npm ci` installation, you can symlink the parent workspace's `node_modules` directly into your worktree project directory (run this command from the worktree's `milo/ui/` folder):
   ```bash
   PARENT_ROOT=$(git worktree list | head -n 1 | awk '{print $1}')
   ln -s "${PARENT_ROOT}/milo/ui/node_modules" node_modules
   ```

3. **Clean Up**:
   Once changes are committed and uploaded to Gerrit, return your parent workspace context and remove the worktree:
   ```bash
   git worktree remove <worktree_path>
   # Also delete the local branch associated with the worktree
   git branch -d <branch_name>
   ```
   *Tip:* If Git encounters stale administrative metadata or path conflicts when creating a new worktree, run `git worktree prune` to clean up all defunct worktree administrative references cleanly.
   > [!WARNING]
   > If the worktree contains untracked build outputs or test artifacts, Git will reject removal. You can run `git worktree remove --force <worktree_path>` **only** after verifying that all your important code changes are committed/uploaded, as `--force` will discard any uncommitted modifications or untracked files within that folder.

### Fallback Workflow (Local Branch Isolation)

If `git worktree` commands are unavailable or restricted (e.g. in restricted sandboxes or headless tasks where permission prompts time out), fallback to local branch isolation.

> [!WARNING]
> **No Concurrent Execution**: The local branch isolation fallback operates within the same shared directory. Unlike Git worktrees, it **does not** support running multiple agent tasks concurrently in the same workspace. Running parallel tasks using this fallback will result in branch conflicts and Git stash corruption (e.g., popping each other's changes). If you must run parallel tasks and worktrees are unavailable, you must run them in separate physical workspace clones.

1. **Save Work Conditionally**: Determine your current branch name first (run `git branch --show-current` and set it as `PARENT_BRANCH="<parent_branch_name>"`). Choose a temporary branch name (set it as `BRANCH_NAME="<temporary_branch_name>"`). If there are uncommitted changes, stash them using a unique, identifiable message containing your temporary branch name so you can retrieve it safely later:
   ```bash
   git stash push -u -m "fallback-stash-${BRANCH_NAME}"
   ```
2. **Branch Off**: Fetch latest from origin, then create and switch to a new branch:
   ```bash
   git fetch
   git checkout -b "${BRANCH_NAME}" origin/main
   ```
3. **Develop & Stage**: Make and stage changes on this branch.
4. **Commit & Upload**: Commit and upload the CL using standard non-interactive flags.
5. **Restore Workspace**: Ensure your temporary branch is clean (either commit all edits or run `git reset --hard HEAD` and `git clean -fd` to remove unwanted modifications/untracked files). Switch back to your parent branch by name and restore your stashed work by finding the specific stash index matching your unique message (only if you stashed work in Step 1):
   ```bash
   # Switch back to the parent branch explicitly
   git checkout "${PARENT_BRANCH}"

   # Find the index of the specific labeled stash
   STASH_REF=$(git stash list | grep "fallback-stash-${BRANCH_NAME}" | head -n 1 | awk -F: '{print $1}')
   if [ -n "$STASH_REF" ]; then
     git stash pop "$STASH_REF"
   fi
   ```
   *Note:* If `git stash pop` fails due to merge conflicts, the stash is preserved. You must resolve the conflicts manually before proceeding.
6. **Clean Up**: Delete the local branch after upload/landing: `git branch -d "${BRANCH_NAME}"`. Only use the force delete flag `-D` if you have explicitly verified that the branch changes have landed or been safely uploaded/pushed to Gerrit.

## Workflow Checklist

Copy this checklist into your response to track execution:

```
Worktree Isolation:
- [ ] Step 1: Check for uncommitted changes (stashing is only required if using the fallback branch workflow)
- [ ] Step 2: Create worktree directory nested inside workspace (`git worktree add`), OR fallback to local branch checkout (`git checkout -b`)
- [ ] Step 3: Change Cwd to new worktree (if worktree created), OR proceed in current directory (if using branch fallback)
- [ ] Step 4: Perform development, linting, and testing inside the isolated environment
- [ ] Step 5: Commit, upload CL, and clean up the worktree (`git worktree remove`) or local branch (`git branch -d`) safely
```

## Guardrails

> [!IMPORTANT]
> - **Worktree Location:** In standard setups, creating Git worktrees at the root of the repository or sibling to the workspace is preferred. However, if the agent is running in a sandboxed execution environment with strict write boundaries, always create the `<worktree_path>` nested *inside* your allowed workspace directory under a dedicated hidden folder such as `.worktrees/` (e.g., `.worktrees/<branch-name>`) to avoid sandbox permission errors. You MUST add `.worktrees/` to your local project `.gitignore` or `.git/info/exclude` so it does not pollute the main index. See [preventing-workspace-leakage](../preventing-workspace-leakage/SKILL.md) for details on staging and ignore hygiene.
> - **Branch Names:** Enforce unique branch names across active worktrees to prevent HEAD reference conflicts.
> - **Symlink Dependency Hazards:** Sharing `node_modules` via symlinks between the parent workspace and worktrees is an optimization meant for read-only linting, type-checking, and running tests.
>   - **Mutation Conflict:** If you run any commands that mutate dependencies (e.g., `npm install`, `npm ci`, `npm update`, or package additions) inside one worktree, it will directly modify the shared `node_modules` in the parent workspace, which can instantly corrupt or break other concurrent worktrees.
>   - **Concurrent Cache Corruptions:** Testing and compilation frameworks (like Jest, Vite, Babel) write local build caches into `node_modules/.cache`. If multiple worktrees run concurrent builds or tests sharing the same symlinked `node_modules`, their caches will conflict, resulting in race conditions or random test flakes.
>   - **Execution Boundaries:** Nested binary execution references under `node_modules/.bin/` or relative path lookups inside dependency loaders can sometimes fail when resolving paths through symlinks.
>   - **Remediation:** If a task requires modifying `package.json`, adding dependencies, or updating structural lockfiles, or if multiple worktrees need to run heavy concurrent test suites, you MUST avoid symlinking `node_modules`. Instead, execute a clean, isolated package installation (`npm ci` or `npm install`) within that specific worktree directory, or configure tool caches to use isolated folders (e.g., using `--cacheDirectory` in Jest, or setting isolated cache env vars), or clone a completely separate workspace.


