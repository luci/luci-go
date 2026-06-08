---
name: coordinating-multiple-repositories
description: Coordinates commit order, branch safety, and submodule updates in workspaces containing nested git repositories. Use this skill when editing code or checking status across parent and submodule boundaries.
---

# Coordinating Multiple Repositories

## Quick Start

To coordinate changes across nested git repositories:

1. **Identify Repository Scope**: Determine if the edited file belongs to the parent repository (e.g., `infra/infra`) or a nested checkout repository (e.g., `luci-go` under `go/src/go.chromium.org/luci`).
   * *Note on Terminology:* In Chromium and `depot_tools` managed environments, nested repositories are typically called "gclient dependencies" or "DEPS checkouts" rather than Git submodules, but they behave similarly when staging (appearing as gitlink adjustments in the parent repository).
2. **Observe Nested Submodule Updates**: Checking out a branch or modifying files in a submodule updates the submodule's Git link pointer in the parent repository.
3. **Surgical Staging**:
   * **Submodule Changes:** Perform git commit and git cl upload directly within the submodule's sub-directory.
   * **Parent Changes:** Stage parent repository files separately. Do NOT commit submodule git links (e.g., `go/src/go.chromium.org/luci` appearing as modified without files) unless you explicitly intend to roll/bump the submodule pointer for the entire project.

## Coordination Checklist

Copy this checklist into your response:

```
Multi-Repo Coordination:
- [ ] Step 1: List all modified files and group them by their repository path boundaries
- [ ] Step 2: Verify submodule commit position matches what is expected in parent project
- [ ] Step 3: Perform commits inside submodule directories first, then upload and land
- [ ] Step 4: Roll/update submodule pointers in the parent repository as a separate, final commit
```

## Guardrails

> [!WARNING]
> - **Accidental Gitlinks:** Running a parent repository `git add .` will accidentally stage submodule pointers (gitlinks) that you didn't mean to touch. Always run `git diff origin/main -- <submodule_path>` before committing parent files to ensure no accidental subproject bumps are included.
> - **Submodule/Dependency Synchronization**: After switching branches, pulling updates, or rebasing in the parent repository, nested repositories may point to outdated references. You MUST perform upfront environment detection before running any synchronization command:
>   1. **Detect gclient/depot_tools Environment**: Check if a `.gclient` or `DEPS` file exists at the root of the parent repository. If yes, execute `gclient sync -d` from the parent root to sync nested repositories according to the configured `DEPS` pins.
>   2. **Detect Standard Git Submodule Environment**: Check if a `.gitmodules` file exists at the root of the parent repository. If yes, execute `git submodule update --init --recursive` to synchronize standard submodules.
>   *This upfront check ensures you do not run incompatible sync tools in the wrong workspace context.*


