---
name: project_verification
description: Instructions for running tests, linter, and type-checks.
---

# Project Verification Skill

> **Note**: This document contains instructions for AI code assistants working in this repository. Human developers can use it as a reference.

Use this skill before declaring a task complete to ensure no regressions.

## Commands

> **Note**: If you get an error that `npm` is not found, you may need to initialize the environment by running `env.py` (refer to the setup instructions in `src/fleet/README.md`).

- **Linting**:
  - `npm run lint`: Runs linting on the entire project.
  - `npm run lint-inc`: Runs linting only on files modified against `origin/main` (faster).
- **Testing**: `npm test -- ./src/fleet/`
- **Type Checking**: `npm run type-check`
