---
name: bypassing-interactive-prompts
description: Bypasses interactive editors and confirmation prompts during command executions in headless background tasks. Use this skill when running git cl upload or git cl description in background loops.
---

# Bypassing Interactive Prompts

## Quick Start

To execute Git commands non-interactively:

1. **Pass Explicit Description via Stdin (Highest Priority)**:
   When uploading or updating a CL, always pass the description explicitly via standard input (stdin) using the `--commit-description -` flag (which implicitly enables force/`-f` mode). This is the safest, most stable, and most deterministic way to bypass editor windows without generating empty descriptions or breaking downstream presubmit rules:
   ```bash
   # Wrap complex pipelines in an explicit bash execution block to guarantee set -o pipefail compatibility
   # regardless of the underlying sandbox runner shell (e.g. standard /bin/sh on Dash/Debian environments)
   bash -c 'set -o pipefail; printf "%s\n" "My initial CL description" | git cl upload --commit-description - -t "My patchset title"'
   ```
   Alternatively, to use the current commit's description without editing, explicitly pass `--commit-description +` combined with the force (`-f`) flag:
   ```bash
   git cl upload -f --commit-description + -t "My patchset title"
   ```
   To update an already uploaded CL's description non-interactively, pass the description via stdin using the `git cl description` command with the `-n` and `-` flags:
   ```bash
   # Wrap in bash -c to ensure POSIX and pipefail compatibility across runner environments
   bash -c 'set -o pipefail; printf "%s\n" "My new CL description" | git cl description -n -'
   ```
2. **Native Non-Interactive Flags**: Check if other git/CLI commands support native non-interactive or bypass flags (e.g., `git cl upload -f` to force bypass description prompts, `--bypass-hooks`, or clean install flags like `npm ci` / `npx -y`).
3. **Direct Patchset Titles**: Always specify patchset descriptions using the `-t` flag:
   ```bash
   git cl upload -t "My patchset title"
   ```
4. **Failing Interactive Editor Intercepts (Bypass Editor Traps)**:
   If a command does not support passing input via stdin/flags and attempts to force open an interactive editor, do NOT override with success overrides like `EDITOR=true` or `EDITOR=touch` (as this can cause commands to succeed silently with empty description text, bypassing Gerrit validations or missing mandatory Bug tags). Instead, set the editor environment variable to a failing interceptor like `EDITOR=false` or `EDITOR="exit 1"`:
   ```bash
   # Force the command to fail-fast if it unexpectedly attempts to open an editor
   EDITOR=false git cl upload
   ```
   This ensures that any failure to bypass prompts is caught early in your task run, allowing you to debug and correct the pipeline parameters rather than masking structural errors.
5. **Task Manager Input Injection:** If a background command stabilizes and prompts for input (e.g., `Get the latest changes and apply on top? [Yes/No]:`), utilize your `manage_task` tool with `Action: send_input` to answer:
   ```
   No
   ```

## Headless Execution Checklist

Copy this checklist into your response:

```
Headless Execution:
- [ ] Step 1: Verify all parameters are passed to the command directly via CLI flags
- [ ] Step 2: Enforce environment overrides (e.g. EDITOR=false) to block interactive loops
- [ ] Step 3: Monitor background task progress and detect prompt states
- [ ] Step 4: Respond to prompts cleanly using send_input pipes
```

## Guardrails

> [!IMPORTANT]
> - **Prompt Detection:** Never loop `status` calls to wait for prompt states. Rely on system notifications and wakeups, then inject standard responses cleanly.
> - **Absolute Prohibition on Piping yes:** Never pipe `yes` inputs (`yes y | ...`) to any commands. Doing so presents a catastrophic risk by blindly authorizing potentially destructive operations (e.g., branch deletion, force-pushing, or untracked file purges). You MUST standardise solely on targeted flags (e.g., `-f`, `--force`, `--bypass-hooks`, `--cherry-pick-stacked`) or use the task manager's `send_input` tool to send individual, validated confirmations.
> - **Shell Fail-Safe (set -e & pipefail):** When wrapping background commands in local shell scripts or invoking sequential git commands inside automated sub-processes, always ensure they execute with `set -e` and `set -o pipefail` (or equivalent shell error handling). Because `set -e` alone does not catch failures that occur inside command pipes (only the exit code of the last command in a pipeline is checked), enabling `set -o pipefail` guarantees that a failure at any stage of a command pipe collapses the script instantly. This ensures any structural failure (e.g., a command in a pipeline failing to bypass prompts) breaks the execution immediately rather than spinning endlessly in a failing headless loop.
>   - **Note on Portability:** `set -o pipefail` is a Bash/Zsh-specific extension and is not supported by standard POSIX `/bin/sh`. Ensure your automation scripts explicitly specify a `#!/bin/bash` or `#!/bin/zsh` shebang rather than `#!/bin/sh` to prevent syntax errors.
> - **Variable Quoting & Injection Safety:** When piping descriptions dynamically (e.g. from variables containing generated text), you MUST use double-quoted variable expansions inside `printf` (e.g., `printf '%s\n' "$DESCRIPTION"`). Never use unquoted expansions (`$DESCRIPTION` without quotes) or unescaped variables as they will cause parameter splitting, parameter expansion, or shell injection vulnerability.

