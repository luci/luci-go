# ADR-002: Static UX Prototyping Architecture and Hosting Boundary

* **Status**: Approved
* **Date**: 2026-08-11
* **Related Documents**: [`mock-api-architecture.md`](mock-api-architecture.md), [`prototypes/README.md`](../prototypes/README.md), [`http://goto.google.com/fcon-labs`](http://goto.google.com/fcon-labs)

## Context

Fleet Console engineers, product managers, and UX designers iterate rapidly on multi-page workflows such as device inventory filtering, repair queues, and resource requests. Sharing interactive UX mocks and recovering their source code later requires:
1. Running real Fleet Console React components in the browser without deploying backend servers.
2. Packaging a self-contained static bundle (`dist_proto/`) that includes the exact source diff (`changes.patch`) and base commit SHA needed to reconstruct the code later.
3. Keeping the open-source repository (`go.chromium.org/luci/milo/ui`) decoupled from any specific internal or external static hosting provider.

## Decision

We establish a two-layer boundary between **static prototype bundle generation** (in open-source `milo/ui/src/fleet/`) and **static bundle hosting**:

1. **Client-Side Mock API (`FleetConsoleMockAPI`)**:
   Prototypes use `FleetConsoleMockAPI` (`src/fleet/testing_tools/mock_api/mock_api_handler.ts`) to intercept `/prpc/fleetconsole.FleetConsole/*` calls in the browser using schema-validated fixtures.

2. **Self-Contained Static Bundle Contract (`dist_proto/`)**:
   A prototype bundle consists of the compiled static assets (`index.html`, `assets/*`, `settings.js`, `ui_version.js`), a reproducible git patch against `origin/main` (`changes.patch`), and a metadata manifest (`.prototype-meta.json` recording `vcs.baseCommitSha`).

3. **Decoupled Hosting**:
   The open-source repository documents how to build and package `dist_proto/` (`src/fleet/.agents/skills/fcon-easy-mock/SKILL.md` and `src/fleet/docs/prototypes/README.md`) without bundling provider-specific uploaders. Developers can serve `dist_proto/` on any static host, and Googlers can publish `dist_proto/` to the internal FCon Labs directory ([`http://goto.google.com/fcon-labs`](http://goto.google.com/fcon-labs)).

## Consequences

* **Minimal Open-Source Footprint**: `luci-go` contains zero internal hosting scripts, credentials, or deployment endpoints.
* **Lossless Code Recovery**: Every static prototype carries `changes.patch` and `vcs.baseCommitSha`, allowing any engineer or agent to apply (`git apply --3way`) and reconcile the prototype against future `main` commits.
* **Host Portability**: The same `dist_proto/` bundle runs on `npx serve`, public static hosts, or internal gallery hosting ([go/fcon-labs](http://goto.google.com/fcon-labs)).
