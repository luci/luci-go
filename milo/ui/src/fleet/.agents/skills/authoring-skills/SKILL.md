---
name: authoring-skills
description: Guides the creation, editing, formatting, and structural best practices for agentic skills. Use this skill when writing or reviewing skill markdown files.
---

# Authoring Skills

## Quick Start

Follow these standards when creating or updating agentic skills in this repository:

1. **Naming Conventions**:
   - Folders must use lowercase, hyphenated naming (spinal-case), e.g., `preventing-workspace-leakage`.
   - The main instruction file must always be named `SKILL.md`.

2. **YAML Frontmatter**:
   - `name`: Must exactly match the directory name.
   - `description`: A clear, concise summary of what the skill does and **exactly when to trigger it**, written in the third person.
     *Example:* `description: Verifies, commits, and uploads code changes as a Gerrit CL. Use when you have completed a task, fixed a bug, or are ready to submit changes for review.`

3. **Checklist Copy Pattern**:
   - If the skill represents a multi-step workflow, you MUST define a **Progress Checklist** block.
   - Mandate that the agent copy this checklist into its very next response to the user and check off items as they complete them to establish execution visibility.

4. **Cross-Referencing**:
   - When a workflow in one skill depends on another skill, explicitly link to that skill's document using relative paths, e.g., [preventing-workspace-leakage](../preventing-workspace-leakage/SKILL.md).
   - This ensures repository portability across different developer environments and user accounts. Do NOT check in absolute paths containing user-specific directories (such as `/Users/`).

5. **Sandbox & Temp File Hygiene**:
   - Advise agents that if their tool executions (like testing, query generation, or log dumps) require writing transient files, they MUST write them strictly to the gitignored project-level `.tmp/` directory (referencing `milo/ui/.tmp/` relative to the repository root) to prevent workspace pollution. Emphasize that `.tmp/` is reserved for short-lived, transient outputs that may be wiped at any time, whereas long-lived local configs or reusable test mocks should be stored under project-level ignored folders like `.idea/`. Clarify that deleting transient files is not required (since `.tmp/` is gitignored) and they can be left there or overwritten with an empty string (`""`) to bypass permission prompts.

6. **Skill Registry Search & Deduplication**:
    - Before authoring a new skill, you MUST list and search the existing skills directory (`src/fleet/.agents/skills/`) to check if a skill with similar functionality or description already exists. Propose to expand or merge into the existing skill manual rather than creating a duplicate.

## Skill Validation Process

Before checking in a new or modified skill, you MUST validate that it works as expected by running it through a subagent:

1. **Design a Verification Task**: Define a realistic development or testing task that triggers the skill.
2. **Define Evaluation Criteria**: Specify target outputs to check (e.g. git status cleanliness, specific log statements, presence of files in `.tmp/`, or screen captures of UI components).
3. **Invoke Subagent**: Spawn a `self` subagent passing it the task.
   - **Workspace Context**: Choose the correct workspace mode:
     - **Inherit (Default)**: Use `Workspace: 'inherit'` if you want the subagent to see your current, uncommitted working tree. **Mandatory Cleanup:** The subagent's task description MUST explicitly command it to run a strict cleanup routine inside a final cleanup step. Running a global destructive clean (like `git clean -fd` at the repository root) inside an inherited parent workspace is strictly prohibited, as it can erase the user's unrelated local uncommitted files or tooling caches. Instead, cleanup routines must target specific, isolated paths (such as restoring modified files via `git restore <path>`). To clean up temporary files in inherited workspaces, always prefer explicit target file paths or lists rather than executing directory-wide destructive sweeps like `git clean -fd` which run the risk of deleting the user's uncommitted scratchpads, config states, or mock parameters.
     - **Branch**: Use `Workspace: 'branch'` to run the task in an isolated branch. Note that you MUST commit your local skill changes first (without uploading to Gerrit) so the branched workspace inherits your updated skill files. (Note: These local commits are transient structural markers to enable branch checkouts, and should not be uploaded/pushed to Gerrit until you are ready to create/update the final CL).
   - **Passing Skill Manuals**: Explicitly provide the absolute paths to the target skill manuals in the subagent's prompt text, and instruct the subagent to read them using the `view_file` tool with the `IsSkillFile` parameter set to the boolean `true` primitive (not a string) before beginning the task.
4. **Inspect Artifacts**: After the subagent completes, immediately check its outputs (such as git patches, logs, or screenshots) to verify the skill rules were followed.
5. **Iterate & Refine**: If the subagent fails or bypasses rules due to environment limits (like sandbox permissions), update the skill to include explicit fallback paths (e.g. branch fallbacks for worktrees) and re-run the validation until it passes.

## Workflow

> [!IMPORTANT]
> **At the start of authoring or editing a skill**, you MUST copy the progress checklist below into your very next response to the user, and check off the steps sequentially as you complete them to ensure structured progress visibility.

```
Progress:
- [ ] Step 1: Ensure directory name matches frontmatter name in spinal-case
- [ ] Step 2: Confirm trigger conditions are explicitly stated in description
- [ ] Step 3: Implement Checklist Copy Pattern for any sequential step execution
- [ ] Step 4: Verify all links to sibling skills are portable relative paths
- [ ] Step 5: Enforce .tmp/ usage for all transient/generated test files
- [ ] Step 6: Search registry directory to verify no duplicate skill already exists
```

## Guardrails

> [!IMPORTANT]
> - **No Placeholders**: Never include generic instructions or empty TODOs in skill manuals.
> - **Sanity Checks**: Always run a linter or verification on the skill Markdown files before committing.
