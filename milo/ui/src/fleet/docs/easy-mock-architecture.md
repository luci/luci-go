# Fleet Console Static Prototype Builder Architecture

The static prototype builder ([`scripts/build_prototype.ts`](../scripts/build_prototype.ts)) compiles the Fleet Console React application into a self-contained static HTML/JS/CSS bundle (`dist_proto/`) that runs offline without a live backend server or authentication flow.

## Overview

When engineers or designers prototype UI changes, waiting on backend RPC implementations or deploying a full backend environment slows down iteration. The static prototype builder produces a portable bundle with relative asset paths (`base: './'`) and in-memory mock RPC handlers so any route under `/ui/fleet/*` can be previewed in a static file host.

## Compilation Pipeline

Running `npm run build:proto` (or `make fcon-easy-mock` from `milo/ui/src/fleet/`) executes `scripts/build_prototype.ts`, which performs five steps:

1. **Git Revision Metadata Injection**: Resolves the current Git commit hash and branch name and injects `VITE_FCON_PROTO_GIT_HASH` and `VITE_FCON_PROTO_GIT_BRANCH` into the build environment.
2. **Prototype EntryBootstrap**: Generates a temporary `.proto_entry_temp.tsx` entry point at `milo/ui/` that calls [`FleetConsoleMockAPI.initPrototypeMode()`](../testing_tools/mock_api/mock_api_handler.ts) before mounting `<App />` and normalizes initial browser URLs so static host subpaths route directly to `/ui/fleet/labs/devices`.
3. **Vite `fcon-easy-mock` Plugin**:
   - **Virtual Env Stubs**: Serves an inline `virtual:env-js` configuration and stubs `/configs.js` requests so the bundle never makes network calls for runtime server configuration.
   - **Minimal App Router Module (`src/App.tsx` `load` hook)**: Replaces `src/App.tsx` at build time with a minimal `createBrowserRouter` wrapper that mounts `fleetRoutes` under `/ui/fleet` inside `FakeAuthStateProvider`, bypassing root-level LUCI Milo routes outside `src/fleet/`.
   - **Deterministic Feature Flags**: Rewrites `createFeatureFlag` default percentages in `src/fleet/config/features.ts` to `100` so newly prototyped features are visible by default in static previews.
4. **Post-Build Asset Sanitization**: Strips external analytics scripts and rewrites absolute `/ui/` asset references to relative `./` paths in `dist_proto/index.html`, then copies `index.html` to `404.html` for single-page application (SPA) fallback routing.
5. **Local Diff Audit**: Compares modified files against `HEAD` (`git diff --name-only HEAD`) and warns if changes touch files outside `milo/ui/src/fleet/`.

## Mock RPC Layer (`FleetConsoleMockAPI`)

In prototype mode, [`FleetConsoleMockAPI`](../testing_tools/mock_api/mock_api_handler.ts) intercepts both `window.fetch` and `XMLHttpRequest` calls targeting `fleetconsole.FleetConsole/*` and `luci.resultdb.v1.*` pRPC endpoints:

- **AIP-160 Filter Evaluation**: `ListDevices` and `CountDevices` parse filter strings with [`parseFilter()`](../utils/aip160/parser/parser.ts) and evaluate the resulting AST against the in-memory device fixtures, supporting boolean `AND`/`OR`/`NOT`, parenthesized expressions, quoted string literals, and `labels."<key>"` lookups.
- **Dynamic Aggregation**: `CountDevices` computes task state (`busy`, `idle`) and device state (`ready`, `needManualRepair`, `needRepair`, `repairFailed`) totals directly from the filtered fixture slice so summary counters stay synchronized with active table filters.
- **Persistent Mutations**: Repack, repair, and resource request updates mutate the in-memory fixture store and persist across reloads via `localStorage`.
