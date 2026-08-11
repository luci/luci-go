# Unified Fleet Console Mock API Architecture (ADR)

## Context
Fleet Console relies on pRPC services served by the Go backend (`infra/fleetconsole`).
Previously, offline prototypes (`go/fcon-prototypes`), Jest unit tests, and E2E tests each created ad-hoc mocks or custom `jest.spyOn` implementations. This led to three major problems:
1. **Schema Drift**: Mock objects in unit tests fell out of sync with protobuf definitions in `service.proto`.
2. **Test Flakiness & Overhead**: Asynchronous Promise mocks in React Query triggered uncontrolled re-render cascades in JSDOM.
3. **Duplicated Work**: Prototyping and test fixtures were maintained separately.

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

## Consequences
- **Zero Schema Drift**: Unit tests and prototypes use the same protobuf structures as production Go servers.
- **Fast, Deterministic Tests**: In-memory fetch interception executes in <0.1ms without external network IO.
- **Unified Maintenance**: Adding a new feature only requires updating one fixture set.
