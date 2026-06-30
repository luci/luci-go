---
name: gerrit-workflows
description: Guidelines and commands for managing stacked CLs, preserving review votes, and querying Gerrit API.
---

# Gerrit Workflows Skill

This skill guides the agent in managing Gerrit CL stacks, preserving review votes, and querying Gerrit metadata efficiently.

## Workflow

> [!IMPORTANT]
> **At the start of a task involving Gerrit CLs**, you MUST copy the progress checklist below into your very next response to the user, and check off the steps sequentially as you complete them.

Progress:
- [ ] Step 1: Request persistent git command permissions
- [ ] Step 2: Configure branch upstream tracking and issue mapping
- [ ] Step 3: Fetch and rebase parent and child branches correctly
- [ ] Step 4: Verify changes do not modify files not already present in the CL's diff (if addressing comments on a +1 CL to preserve votes)
- [ ] Step 5: Upload parent CL from parent branch
- [ ] Step 6: Upload downstream CL from child branch

## 1. Preserving Review Votes (Gerrit +1)

> [!IMPORTANT]
> Uploading **any** new patchset to a CL will reset its Gerrit `Code-Review` votes.
> - If a reviewer has already granted `Code-Review+1` and requests non-trivial changes, do not upload a new patchset to that CL unless you want to discard the +1 vote.
> - Instead, address the feedback in a new downstream CL parented on the original CL to maintain review velocity.

## 2. Managing & Uploading CL Stacks

* When working with stacked CLs (e.g., CL B depends on CL A), configure git upstream branch tracking:
  ```bash
  # While on branch CL-B:
  git branch --set-upstream-to=CL-A-branch
  ```
* **Rebasing Stacked Branches**:
  When a parent branch (CL-A) is rebased or amended, a simple `git rebase CL-A-branch` on the child branch (CL-B) can trigger merge conflicts because git tries to re-apply the old parent commits.
  To rebase safely, use one of the following methods:

  **Method A: `git rebase-update` (Recommended)**
  This depot_tools utility automatically and cleanly rebases all local stacked branches on their respective upstreams:
  ```bash
  git rebase-update
  ```

  **Method B: Explicit Rebase Onto**
  If you need to rebase manually, specify the boundary to skip the old parent commits:
  ```bash
  # While on CL-B-branch:
  git rebase --onto CL-A-branch <old-CL-A-commit>
  ```
  where `<old-CL-A-commit>` is the commit hash of the parent CL before it was rebased/amended.
  If conflicts arise during rebase:
  1. Resolve conflicts in the files.
  2. Stage the resolved files: `git add <files>`
  3. Continue rebase: `git rebase --continue` (never create new commits during rebase).

* **Linking to an Existing CL**:
  If a branch has already been uploaded but lost its association, link it to the issue:
  ```bash
  git cl issue <issue_number>
  ```

* **How to Upload Stacked CLs**:
  To prevent Gerrit from getting confused about relation chains:
  1. Checkout the parent branch (CL A).
  2. Run upload: `git cl upload -f --commit-description=+`
  3. Checkout the child branch (CL B).
  4. Run upload: `git cl upload -f --commit-description=+`

* **Cleaning Up Accumulating Commits (Unstacking & Decoupling Branches)**:
  When rebasing local branches that were previously stacked, a branch may carry over commits and file modifications from its old parent branches, causing `git cl upload` to pull in unrelated changes.
  To surgically clean up a branch so it contains exactly one commit with only the desired changes:
  1. Checkout `origin/main` to a temporary branch:
     ```bash
     git checkout -b temp-clean-branch origin/main
     ```
  2. Checkout only the files belonging to the specific CL from your dirty local branch:
     ```bash
     git checkout <dirty-branch-name> -- path/to/file1.tsx path/to/file2.ts
     ```
  3. Commit the clean files:
     ```bash
     git commit -m "[Category] Commit description message"
     ```
  4. Switch back to your dirty branch and hard reset it to the clean state:
     ```bash
     git checkout <dirty-branch-name> && git reset --hard temp-clean-branch
     ```
  5. Delete the temporary branch:
     ```bash
     git branch -D temp-clean-branch
     ```
  This guarantees the branch contains exactly one commit on top of `origin/main` with no unrelated tracking files.

## 3. Preserving Git Rename Tracking

* When moving or renaming source or test files, keep the file's inner content as close to the original as possible (aim for >90% similarity index) in the renaming commit. This ensures Git's rename detection tracks the history correctly instead of treating it as a deletion and a new file, which simplifies code reviews.
* Avoid introducing heavy structural refactoring in the same commit as the file move. Perform renames in a dedicated commit first, then apply clean refactoring in subsequent commits.

## 4. Querying Gerrit API (CLI Efficiency)

Do not use browser sessions or browser subagents to fetch Gerrit metadata. Instead, use the authenticated Gerrit REST API with `curl` using the `--netrc` flag (to load credentials from `~/.netrc`) and the `/a/` prefix.

> [!NOTE]
> The examples below use the standard host `chromium-review.googlesource.com`. Adjust the domain accordingly if the repository is configured for a different Gerrit host.

* **Retrieve Related Changes (Relation Chain)**:
  ```bash
  curl -s --netrc "https://chromium-review.googlesource.com/a/changes/<issue_number>/revisions/current/related" | tail -n +2 | jq .
  ```
* **Retrieve CL Detail**:
  ```bash
  curl -s --netrc "https://chromium-review.googlesource.com/a/changes/<issue_number>/detail" | tail -n +2 | jq .
  ```

> [!NOTE]
> The `tail -n +2` is required to strip Gerrit's anti-XSS magic prefix (`)]}'`) from the beginning of the JSON response, ensuring JSON parsers (or `jq`) do not fail.

> [!WARNING]
> If a `curl` call returns a `401 Unauthorized` or redirect page, verify that `~/.netrc` contains valid credentials for `chromium-review.googlesource.com`. If missing or expired, ask the user to regenerate their password at https://chromium-review.googlesource.com/new-password.

## 5. Requesting Git Permissions

At the start of any session involving git operations, request persistent prefix permission for the `git` command using the `ask_permission` tool:
* **Action**: `command`
* **Target**: `git`
This allows stack management and uploads to run without repeatedly prompting the user.
