---
name: project-verification
description: Runs project checks including linter, tests, and type-checks to ensure no regressions. Use before committing changes, before uploading a CL, or when validating code correctness.
---

# Project Verification Skill

> **Note**: This document contains instructions for AI code assistants working in this repository. Human developers can use it as a reference.

Use this skill before declaring a task complete to ensure no regressions.

## Workflow

> [!IMPORTANT]
> **At the start of project verification**, you MUST copy the progress checklist below into your very next response to the user, and check off the steps sequentially as you complete them.

Progress:
- [ ] Step 1: Run linting (`npm run lint-inc` or `npm run lint`)
- [ ] Step 2: Run tests (`npm test -- ./src/fleet/` or specific spec files)
- [ ] Step 3: Run type checking (`npm run type-check`)

## Commands

> [!NOTE]
> If you get an error that `npm` is not found, you may need to initialize the environment by running `env.py` (refer to the setup instructions in `src/fleet/README.md`).

- **Linting**:
  - `npm run lint`: Runs linting on the entire project.
  - `npm run lint-inc`: Runs linting only on files modified against `origin/main` (faster).
- **Testing**: `npm test -- ./src/fleet/`
- **Type Checking**: `npm run type-check`

## Preventing Test Flakiness and State Leakage

Persistent client-side state (like IndexedDB, LocalStorage, or Cookies) can leak between Cypress specs if not cleared, causing test flakiness (e.g., `cy.wait` timing out because React Query loaded values from IndexedDB instead of making a network request).

- **Automatic Cleanup**: Cypress support at `cypress/support/e2e.ts` is configured to automatically clear IndexedDB, LocalStorage, and Cookies before every test across all specs.
- **Writing New Specs**: Always assume a completely blank client state for every test. Do not rely on state persisting from previous steps or specs. If writing or updating E2E tests:
  - Ensure you mock all network requests (`mockPrpc`) explicitly.
  - Avoid hardcoded timeouts in `cy.wait()`; let standard waits manage requests.
  - **Mandatory Stress Testing**: When authoring new E2E tests or modifying existing ones, you MUST run local stress testing using the `stress_e2e.sh` script to verify they are 100% robust and non-flaky.
    - **Execution Directory**: All E2E/Cypress-related testing and script executions MUST be run from the `milo/ui` directory context (e.g., `cd milo/ui` first).
    - Run at least 5–10 iterations on the specific spec file.
    - Use an isolated `PREVIEW_PORT` to avoid conflicts with default or active running servers.
    - Example command:
      ```bash
      # Change to the milo/ui directory first
      cd milo/ui

      # Run the stress test
      STRESS_COUNT=5 STRESS_SPEC="cypress/e2e/fleet/android_devices_page.cy.ts" PREVIEW_PORT=8765 ./scripts/stress_e2e.sh
      ```

## Local Debugging and Temp Files

If you need to redirect command output, dump test logs, or write intermediate debug files:
- **Rule**: Always write these files to the local gitignored `.tmp/` folder (e.g., `milo/ui/.tmp/`). Never dump them directly in the `src/` or root directories where they could be accidentally staged.
