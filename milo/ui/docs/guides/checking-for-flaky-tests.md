# Checking for Flaky or Slow Tests & Flake Prevention Playbook

This guide explains how to use the canonical `check-tests` program to identify flaky and slow tests, and details the engineering standards required to prevent test flakes in the LUCI UI codebase.

## 1. Overview

The `check-tests` program runs a specified test suite multiple times and reports on failures and performance metrics. It serves as our canonical tool to detect intermittent failures (flakes) and identify slow unit tests before code merges.

---

## 2. Usage

Run `check-tests` from the `milo/ui` directory:

```bash
make check-tests <test_matcher> [options]
```

Or run the script directly from the root of the `luci-go` repository:

```bash
go run milo/ui/scripts/check_tests.go <test_matcher> [options]
```

### Arguments

* `<test_matcher>`: (Required) A string or file path matching the test suite(s) you want to run (e.g. `src/fleet/pages/device_list_page/browser/browser_devices_page.test.tsx`).

### Options

* `-h, --help`: Show the help message.
* `-v, --verbose`: Show full output and stack traces of each test run.
* `-n, --runs`: The number of times to run the test (default: `50`).
* `-p, --perf`: Collect and display average, p50, p90, and p99 performance metrics.

### Example

To run a test suite 50 times with performance profiling:

```bash
make check-tests src/fleet/pages/device_list_page/browser/browser_devices_page.test.tsx -- -n 50 -p
```

---

## 3. Engineering Standards for Flake Prevention

### 3.1 Asynchronous Element Queries
* **Never use synchronous `getBy*` immediately after asynchronous triggers**: Use `await screen.findBy*` when waiting for elements that render after network queries or microtask ticks.
* **Keep `waitFor` pure**: Never trigger user actions (clicks, typing) inside `waitFor` callbacks. Place only assertions inside `waitFor(() => { expect(...).toBe(...) })`.

### 3.2 TanStack Query & Cache Isolation
* **Provider Wrapping**: Always wrap components under test with `<FakeContextProvider>` (`src/testing_tools/fakes/fake_context_provider.tsx`).
* **Query Retries**: Ensure `queries: { retry: false }` is set in test `QueryClient` instances to prevent exponential backoff delays.

### 3.3 Centralized Mocking with `FleetConsoleMockAPI`
* **Avoid Fragmented Network Spies**: Never write ad-hoc `jest.spyOn(client, ...)` or custom `mockFetchRaw` mocks in individual test files.
* **Use `FleetConsoleMockAPI`**: Use `FleetConsoleMockAPI.enableBrowserInterceptor()` and configure fixtures with `FleetConsoleMockAPI.setFixture(method, data)`. This provides deterministic responses while maintaining schema alignment with backend protobuf contracts.

### 3.4 Cleanup & Teardown
* **Always call `jest.restoreAllMocks()`**: In `afterEach`, restore all spies to prevent mock leakage across test suites.
* **Clean Timers**: When using `jest.useFakeTimers()`, advance timers using `await jest.advanceTimersByTimeAsync(...)` to flush microtasks, and always call `jest.useRealTimers()` in `afterEach`.
* **Clean Blob URLs**: Mock `URL.createObjectURL` and `URL.revokeObjectURL` in file export tests.

---

## 4. Flake Diagnostic Workflow

When a test flakes or times out on CI (`luci-go-try-frontend`):

```text
1. Reproduce:
   make check-tests <test_path> -- -n 50 -p -v

2. Diagnose Failure Mode:
   - Exceeded Timeout: Check for unmocked async promises, missing await, or synchronous getBy* calls.
   - Assertion Mismatch: Check for mock leakage across tests (missing jest.restoreAllMocks).
   - Open Handles: Check for unstopped setInterval timers or active subscriptions.

3. Fix:
   - Use FleetConsoleMockAPI fixtures.
   - Stub unrelated background polling components (e.g. AdminTasksAlert).
   - Use findBy* for async DOM resolution.

4. Verify:
   Re-run: make check-tests <test_path> -- -n 50 -p
   Requirement: 50 passed out of 50 runs with zero failures.
```

---

## 5. Pre-Merge Verification Checklist

Before submitting a CL or closing a flake bug:

- [ ] Passed 50 consecutive runs via `make check-tests <test_file> -- -n 50 -p`.
- [ ] Centralized mocks configured via `FleetConsoleMockAPI`.
- [ ] No unhandled Promise rejections or open timer handles.
- [ ] `jest.restoreAllMocks()` called in `afterEach`.
- [ ] Presubmit checks verified via `autorepair investigate --cl <cl_number>`.
