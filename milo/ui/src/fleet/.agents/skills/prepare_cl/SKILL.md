---
name: prepare_cl
description: Instructions for preparing a CL for upload, including committing, verification, and optional upload.
---

# Prepare CL Skill

> **Note**: This document contains instructions for AI code assistants working in this repository. Human developers can use it as a reference.

Use this skill when you have completed a task and need to prepare the changes for code review.

## Workflow

Progress:
- [ ] Step 1: Verification
- [ ] Step 2: Commit Changes
- [ ] Step 3: Optional Upload

## Procedures

1. **Verification**:
   - Run the `project_verification` skill (lint, test, type-check) to ensure no regressions.
   - Run the `senior_reviewer` skill to get a self-review and address feedback.

2. **Commit Changes**:
   - Run `git status` to see modified files.
   - Stage changes: `git add <files>`.
   - Commit changes with a descriptive message: `git commit -m "..."`.

3. **Optional Upload**:
   - Attempt to run `git cl upload`.
   - *Note*: If this fails with an error like `chmod ~/.sso: operation not permitted` or a path-specific permission error, it is due to sandbox restrictions preventing the agent from modifying home directory files. In this case, do not block the task; notify the developer and ask them to run `git cl upload` on your behalf.
