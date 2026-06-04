---
name: preventing-workspace-leakage
description: Detects and prevents untracked, generated, and sandbox-specific files from polluting the Git index. Use this skill before staging changes or reviewing Git status for committing.
---

# Preventing Workspace Leakage

## Quick Start

To prevent sandbox and local test pollution from reaching Gerrit:

1. **Identify Untracked Files**: Run `git status` and look under the "Untracked files" section.
2. **Register Untracked Patterns**: If you see generated or temporary directories or local build artifacts that are not ignored, add them to the local `.gitignore` file immediately. Always prefer writing transient outputs (such as test spools or log dumps) to the project's gitignored `.tmp/` folder to avoid polluting the index.
3. **Perform Staging Isolation**: Stage files by their exact, descriptive name rather than using blanket staging commands like `git add .` or `git add -A`.
   - **Repository Boundaries**: In workspaces containing nested repositories (like the `luci` repository nested inside `infra`), all git commands must be run within the specific repository tracking the files (referred to as the "repository root", e.g., `<workspace_root>/infra/go/src/go.chromium.org/luci`). Running git commands from the parent workspace root will fail to stage or track changes in the nested repository.
   - **Paths**: Use paths relative to the specific repository root, or relative to your current working directory (`Cwd`) if executing inside sub-directories.
     *Example (from repository root `infra/go/src/go.chromium.org/luci`):* `git add milo/ui/cypress/support/e2e.ts`
     *Example (from milo/ui directory):* `git add cypress/support/e2e.ts`

## Staging Checklist

Copy this checklist into your response:

```
Staging Hygiene:
- [ ] Step 1: Run git status to check for untracked or modified files
- [ ] Step 2: Verify all listed untracked files are either expected features or added to .gitignore
- [ ] Step 3: Stage files surgically by path rather than blanket catch-alls
- [ ] Step 4: Run git diff --staged to verify only intended lines are being committed
```

## Guardrails

> [!CAUTION]
> - **Temporary and Sandbox Directories:** Never commit temporary files or local mock inputs. If you need to test mock resources locally but want to avoid commit contamination:
>   - **Transient Files (`.tmp/`)**: Write transient sandbox outputs (such as patches, temporary test runs, and one-off logs/dumps) strictly into the `.tmp/` folder (e.g., `milo/ui/.tmp/`). Note that `.tmp/` is reserved for short-lived items that may be manually or automatically wiped clean at any time. To avoid prompting the user with permission popups for `rm` commands, you can simply leave these files there or overwrite them with an empty string (`""`) using `write_to_file`.
>   - **Long-lived Local Files (`.idea/` or `.vscode/`)**: If you need to write persistent local configurations, persistent mock assets, or scratchpads that you expect to reuse across sessions but do not want to track, store them under project-level ignored tool folders like `.idea/` (or `.vscode/`).
>   - For **tracked files** with local modifications, use `git update-index --skip-worktree <file_path>` so local changes are ignored and not staged. If you later need to stage changes to these files, restore standard tracking using `git update-index --no-skip-worktree <file_path>`.
> - **Ignore Hygiene:** Keep project-local `.gitignore` updated with any new testing and toolchain paths generated during development.


