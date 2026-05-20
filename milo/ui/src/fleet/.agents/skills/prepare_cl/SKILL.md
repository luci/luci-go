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
- [ ] Step 3: Upload UI Demo (Optional/Conditional)
- [ ] Step 4: Optional Upload

## Procedures

1. **Verification**:
   - Run the `project_verification` skill (lint, test, type-check) to ensure no regressions.
   - Run the `senior_reviewer` skill to get a self-review and address feedback.

2. **Commit Changes**:
   - Run `git status` to see modified files.
   - Stage changes: `git add <files>`.
   - Commit changes with a descriptive message.
     - **Commit Message Guidelines**:
       - Title: Short, descriptive summary.
       - Body: Explain what changed and why. If it fixes a bug, explain the fix.
       - Footer: Include bug reference in the format `Bug: b/XXXXXXX` (note the `b/` prefix).
     - Example: `git commit -m "[fleet] Fix CSV export missing columns by preserving URL columns\n\n... details ...\n\nBug: b/515102813"`

3. **Upload UI Demo**:
   - For UI changes, consider uploading a demo to a dev environment.
   - **Judgment Call**: Upload a demo if the change is non-trivial and would benefit from visual verification by a human reviewer.
   - Run `make deploy-ui-demo` in the UI directory.
   - *Note*: If this fails with auth errors (e.g., `Login first using 'gcloud auth login'`), notify the developer and ask them to run it on your behalf. Do not block the task on this failure. (Alternatively, the developer may choose to disable the sandbox to allow direct uploads).
   - If successful, include the demo link in the CL description.
   - Include testing instructions for the reviewer in the CL description, encouraging them to also test edge cases or related functionality that might break.

4. **Optional Upload**:
   - Attempt to run `git cl upload`.
   - *Note*: If this fails with an error like `chmod ~/.sso: operation not permitted` or a path-specific permission error, it is due to sandbox restrictions preventing the agent from modifying home directory files. In this case, do not block the task; notify the developer and ask them to run `git cl upload` on your behalf. (Alternatively, the developer may choose to disable the sandbox to allow direct uploads).


