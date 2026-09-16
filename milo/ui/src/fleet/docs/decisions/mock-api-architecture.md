# Unified Fleet Console Mock API Architecture (ADR)

## Context
Fleet Console relies on pRPC services served by the Go backend (`infra/fleetconsole`).
Previously, offline prototypes (`go/fcon-prototypes`), Jest unit tests, and E2E tests each created ad-hoc mocks or custom `jest.spyOn` implementations. This led to three major problems:
1. **Schema Drift**: Mock objects in unit tests fell out of sync with protobuf definitions in `service.proto`.
2. **Test Flakiness & Overhead**: Asynchronous Promise mocks in React Query triggered uncontrolled re-render cascades in JSDOM.
3. **Duplicated Work**: Prototyping and test fixtures were maintained separately.

## Twofold Goals of Mock Infrastructure
This architecture serves two explicit goals:
1. **Unblock Frontend Prototyping Pipeline (`http://go/fcon-prototypes`)**:
   Provide a production-like synthetic mock data generator and network interceptor layer between Fleet Console frontend code and backend APIs. This allows hosting static-only frontend prototypes using real Fleet Console UI components with full visual and functional fidelity without needing live backend server connections.
2. **Front Door Integration Testing**:
   Utilize the synthetic mock datasets to drive automated integration tests that interact with Fleet Console from the "front door" (testing real React components, pages, and hooks as a real user would experience them).

## Decision
We establish **`FleetConsoleMockAPI`** (`src/fleet/testing_tools/mock_api/mock_api_handler.ts`) as the single source of truth for all pRPC network mocking across:
1. Live static browser prototypes (`go/fcon-prototypes`).
2. Jest & Vitest unit and integration tests.
3. Cypress and Playwright end-to-end smoke tests.

### Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          Unified Mock API Lifecycle                         │
└─────────────────────────────────────────────────────────────────────────────┘
  Component / Test / Prototype
        │
        ▼
  usePrpcServiceClient ➔ PrpcClient ➔ globalThis.fetch
        │
        ▼
  FleetConsoleMockAPI Interceptor (globalThis.fetch)
        ├── Intercepts /auth/openid/state ➔ Returns DEFAULT_AUTH_STATE
        └── Intercepts /prpc/fleetconsole.FleetConsole/*
              ├── Retrieves schema-compliant fixture (DEFAULT_FIXTURES or setFixture)
              └── Returns Response with LUCI XSSI prefix ( )]}'\n )
```

### Key Principles

1. **Mandatory Synchronization Rule (`AGENTS.md`)**:
   Whenever a pRPC RPC method is added, modified, or deprecated in `service.proto`, the corresponding fixture in `mock_api_handler.ts` must be updated in the same change.
2. **Universal Interception**:
   Calling `FleetConsoleMockAPI.enableBrowserInterceptor()` intercepts `globalThis.fetch` globally without requiring changes to components or hooks.
3. **Deterministic Test Overrides**:
   Tests override fixtures synchronously before mounting components using:
   ```typescript
   FleetConsoleMockAPI.setFixture('ListBrowserDevices', { devices: [...], totalSize: 2 });
   ```
4. **Clean Isolation**:
   `FleetConsoleMockAPI.resetFixtures()` restores defaults between test cases in `beforeEach`.
5. **Front Door Testing Principle**:
   Automated tests should exercise Fleet Console from the front door (mounting top-level page components or React Query custom hooks directly against `FleetConsoleMockAPI`) rather than mocking React component internals or `jest.spyOn` on internal modules. All checked-in sample mock datasets in `src/fleet/testing_tools/mock_api/data/*.json` MUST be actively consumed in front-door integration tests—no unused or dead mock data files may be checked into source control.
6. **Standalone Prototyping Support**:
   Static prototypes (hosted on Cloud Storage or static dev servers) can initialize network-level simulation with state persistence and latency control:
   ```typescript
   FleetConsoleMockAPI.initPrototypeMode({
     persistToLocalStorage: true, // Persists mutations across browser reloads
     latencyMs: 150,              // Simulates realistic network response delay
     authState: { email: 'proto-admin@google.com' },
   });
   ```
7. **Synthetic Mock Data Generation**:
   We provide a deterministic mock data generator script (`src/fleet/scripts/generate_mock_data.ts`, runnable directly via `./src/fleet/scripts/generate_mock_data.ts`).
   - **Convenience for Developers**: The script allows team members to easily edit distributions, add edge cases, or scale data counts (via `--full`) when exploring prototypes.
   - **Sample Fixtures in Source**: Concise, representative sample datasets are checked into source control (`src/fleet/testing_tools/mock_api/data/*.json`) so offline prototypes and automated tests run immediately without manual setup.
   - **Synthetic Placeholders**: All generated records use generic placeholder names with explicit `demo-`, `mock-`, or `sim-` prefixes for offline prototyping and test isolation.

## Consequences
- **Zero Schema Drift**: Unit tests and static prototypes use the same protobuf structures as production Go servers.
- **High-Fidelity Prototyping**: Static UI prototypes support full interactive CRUD user flows (editing inventory, reserving devices, running repairs) with optional `localStorage` persistence.
- **Front-Door Test Coverage**: Integration tests validate complete frontend user journeys through the real UI layer down to network interception.
- **Fast, Deterministic Tests**: In-memory fetch interception executes in <0.1ms without external network IO.
- **Unified Maintenance**: Adding a new feature only requires updating one fixture set.

