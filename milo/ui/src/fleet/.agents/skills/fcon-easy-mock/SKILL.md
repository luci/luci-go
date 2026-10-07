---
name: fcon-easy-mock
description: Instructions for building and packaging standalone static Fleet Console UI prototype bundles (dist_proto/) with client-side mock data and reproducible git patches for any static host.
---

# FCon Easy Mock Skill (`fcon-easy-mock`)

> **Note**: This document contains instructions for AI agents and developers working in `milo/ui/src/fleet/`.

Use this skill when building, packaging, or sharing a **standalone static Fleet Console UX prototype** (`dist_proto/`) that runs in the browser using client-side mock data (`FleetConsoleMockAPI`) without deploying backend services.

> [!IMPORTANT]
> **When to Use Static Prototyping (`fcon-easy-mock`) vs. Standard CL Demos (`deploy-ui-demo`)**:
> - Use `fcon-easy-mock` for **multi-page UX explorations, major layout redesigns, or features requiring mocked RPCs that do not exist in the backend yet**.
> - For **routine frontend fixes, visual polish, column/filter additions, and 1-CL changes (`<= 150` LOC)**, follow the standard CL workflow ([prepare-cl](../prepare-cl/SKILL.md) + [deploy-ui-demo](../deploy-ui-demo/SKILL.md)).

---

## Architecture Boundary

Fleet Console static prototyping separates **open-source bundle generation** from **static asset hosting** (see [`easy-mock-architecture.md`](../../docs/easy-mock-architecture.md)):

1. **Open-Source Fleet Console (`milo/ui/src/fleet/`)**:
   - Owns the UI components, routes, and [`FleetConsoleMockAPI`](../../testing_tools/mock_api/mock_api_handler.ts), which intercepts `/prpc/fleetconsole.FleetConsole/*` and `/auth/openid/state` requests in the browser.
   - Compiles a self-contained static directory (`dist_proto/`) via [`scripts/build_prototype.ts`](../../scripts/build_prototype.ts).
2. **Static Hosting (Bring Your Own Host)**:
   - The generated `dist_proto/` directory is host-agnostic and can be served from any local or remote static HTTP server (`npx serve dist_proto`, GitHub Pages, Firebase Hosting, Netlify, S3, or GCS).

---

## Step-by-Step Instructions: Building and Previewing a Prototype

### Step 1: Compile the Static UI Bundle (`dist_proto/`)
From `milo/ui/src/fleet/`, run the static prototype builder target:

```bash
make fcon-easy-mock
```

Or run the script directly from `milo/ui/`:

```bash
npx tsx src/fleet/scripts/build_prototype.ts
```

This script automatically:
- Bootstraps [`FleetConsoleMockAPI.initPrototypeMode()`](../../testing_tools/mock_api/mock_api_handler.ts) before mounting `<App />`.
- Stubs `src/App.tsx` with a minimal router that mounts `fleetRoutes` under `/ui/fleet` inside `FakeAuthStateProvider`.
- Enables feature flags in `src/fleet/config/features.ts` so prototype UI is visible by default.
- Emits relative asset paths (`./assets/*`) and `dist_proto/404.html` for single-page application (SPA) fallback routing.

### Step 2: Export the Reproducible Source Patch (`changes.patch`)
When sharing a prototype bundle, include a git diff against `origin/main` inside `dist_proto/changes.patch` so any developer or agent can reconstruct and iterate on your prototype source code:

```bash
git add -N -- src/fleet
git diff "$(git merge-base origin/main HEAD)" -- src/fleet > dist_proto/changes.patch
```

### Step 3: Preview Locally or Upload to a Static Host
Verify that `dist_proto/` contains the compiled bundle:
- `dist_proto/index.html`, `dist_proto/404.html`, and `dist_proto/assets/*`

Preview locally:
```bash
npx serve dist_proto
```
Then upload the `dist_proto/` directory to your static host.

---

## UX Guardrails for Prototypes
- **Use Native Navigation**: Do not inject floating prototype bottom navbars; use the native Fleet Console sidebar and header.
- **Follow UX & Writing Guidelines**: Adhere to [writing-and-ux-principles](../writing-and-ux-principles/SKILL.md) and [high-density-ui](../high-density-ui/SKILL.md).

## Related Skills
- [ux-prototyping](../ux-prototyping/SKILL.md)
- [deploy-ui-demo](../deploy-ui-demo/SKILL.md)
- [high-density-ui](../high-density-ui/SKILL.md)
