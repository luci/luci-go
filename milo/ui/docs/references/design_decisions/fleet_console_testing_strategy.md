# Fleet Console Testing Strategy

## Context
This document outlines the strategy for testing the Fleet Console UI, focusing on balancing test reliability, local developer experience, and CI integration. We evaluate different mocking approaches for Cypress E2E tests to ensure components like the filter bar and device lists are verified against regressions.

## Evaluated Approaches

### 1. Network Mocking with `cy.intercept`
Stubbing pRPC requests directly within Cypress tests.
*   **Status**: Current implementation.
*   **Pros**: Runs entirely within Cypress without external dependencies. Fast and reliable when protocol requirements are met.
*   **Cons**: Requires maintaining mocks that match complex proto shapes and protocol details (e.g., XSSI prefix `)]}'\n` and `X-Prpc-Grpc-Code` header).

### 2. Mock Service Worker (MSW)
Using a Service Worker to intercept network requests at the browser level.
*   **Status**: Evaluated and deferred.
*   **Pros**: Allows stateful mocks (maintaining state across requests); can be shared between Cypress, Storybook, and unit tests; uses standard fetch API (Request/Response); provides better isolation by running in a separate worker thread.
*   **Cons**: Encountered issues with request interception inside the Cypress iframe environment and dependency resolution blocks in the current environment.

### 3. Full-Stack Integration in CI
Running a real backend pointing to a test database in CI.
*   **Status**: Future goal.
*   **Pros**: Most reliable; eliminates mock maintenance.
*   **Cons**: Docker execution constraints on some developer environments (e.g., Macbooks) make local reproduction challenging.

## Decisions & Future Plan

### Current Decision
We continue to use **Option 1 (`cy.intercept`)** for localized network mocking. To prevent race conditions and loading state hangs, all mocks must strictly follow the pRPC protocol requirements (XSSI prefix and appropriate headers).

To minimize boilerplate, tests should use helper functions to wrap the XSSI prefixing and header injection.

### Why MSW is Deferred
While MSW provides stateful mocking and better Storybook alignment, it is currently deferred because:
1.  **Iframe Isolation**: Cypress runs tests in an iframe, which isolates requests from the Service Worker registered in the main window. Workarounds to register the worker in the iframe caused browser crashes in our environment.
2.  **Dependency Blocks**: Installing MSW required navigating Skia audit mirror rules (e.g., package age requirements), adding friction to the setup.

Future developers are encouraged to revisit MSW if they can resolve the iframe interception issue reliably without causing browser instability.

### Future Work: Moving E2E tests to Presubmit
Currently, E2E tests are only run in the `luci_ui_promoter` builder during the promotion pipeline, not as blocking tryjobs on CLs. This creates a risk where broken tests can land and block deployment.

Future work should consider:
1.  **Adding a Try Builder**: Creating a new try builder or updating `luci-go-try-frontend` to run `make e2e`.
2.  **Optimizing Latency**: Ensuring E2E tests run fast enough to not degrade CQ performance, possibly by running them in parallel or filtering by modified paths.

### Testing Pyramid: When to use what?
LUCI platform chose **Cypress** as the standard for E2E testing. Follow these guidelines for choosing the right test type:

-   **E2E Tests (Cypress)**: Use for full-page integration, verifying complex user flows spanning multiple components, and ensuring navigation works as expected.
-   **Component/Unit Tests (React Testing Library / Jest)**: Use for isolated component behavior, testing hooks, and pure business logic. Do not use Cypress for things that can be covered by unit tests.

## Guidelines for Agents
Agents MUST run E2E tests headlessly (`npm run e2e` or `npx cypress run`) and ensure they pass before considering any feature complete or ready for upload. Bypassing presubmits is discouraged unless the failure is explicitly unrelated to the modified files.
