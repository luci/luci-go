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

Fleet Console static prototyping separates **open-source bundle generation** from **static asset hosting**:

1. **Open-Source Fleet Console (`milo/ui/src/fleet/`)**:
   - Owns the UI components, routes, and `FleetConsoleMockAPI` (`src/fleet/testing_tools/mock_api/mock_api_handler.ts`), which intercepts `/prpc/fleetconsole.FleetConsole/*` and `/auth/openid/state` requests in the browser.
   - Produces a self-contained directory (`dist_proto/`) containing the compiled static assets, a reproducible source diff (`changes.patch`), and a metadata manifest (`.prototype-meta.json`).
2. **Static Hosting (Bring Your Own Host or [go/fcon-labs](http://go/fcon-labs))**:
   - The generated `dist_proto/` directory is host-agnostic and can be served from any local or remote static HTTP server.
   - **External / Open-Source Developers**: Serve locally with `npx serve dist_proto` or upload `dist_proto/` to any static host (GitHub Pages, Firebase Hosting, Netlify, S3, or GCS).
   - **Googlers**: Upload `dist_proto/` to the internal FCon Labs prototype directory at [`http://go/fcon-labs`](http://go/fcon-labs) ([`http://go/fcon-prototypes`](http://go/fcon-prototypes)) using the internal Google3 publishing skill.

---

## Step-by-Step Instructions: What to Build and Package

### Step 1: Enable Client-Side Mock API Interception
Ensure your prototype views or entry point initialize [`FleetConsoleMockAPI`](../../testing_tools/mock_api/mock_api_handler.ts) (see [`mock-api-architecture.md`](../../docs/decisions/mock-api-architecture.md)):

```ts
import { FleetConsoleMockAPI } from '@/fleet/testing_tools/mock_api';

FleetConsoleMockAPI.initPrototypeMode({
  persistToLocalStorage: true,
});
```

### Step 2: Compile the Static UI Bundle (`dist_proto/`)
From `milo/ui/`, run a Vite production build with relative asset paths (`--base=./`):

```bash
npx vite build --base=./ --outDir=dist_proto
```

Ensure `dist_proto/settings.js` and `dist_proto/ui_version.js` exist and are referenced via relative paths (`./settings.js` and `./ui_version.js`) in `dist_proto/index.html`, with `crossorigin` attributes removed so static file hosts do not block script loads.

### Step 3: Export the Reproducible Source Patch (`changes.patch`)
Include a complete git diff against `origin/main` inside `dist_proto/changes.patch` so any developer or agent can reconstruct and iterate on your prototype source code even after upstream changes:

```bash
git add -N -- src/fleet
git diff "$(git merge-base origin/main HEAD)" -- src/fleet > dist_proto/changes.patch
```

### Step 4: Write the Prototype Manifest (`.prototype-meta.json`)
Write `dist_proto/.prototype-meta.json` with metadata and the base commit SHA:

```json
{
  "name": "<slug>",
  "title": "<Human-Readable Title>",
  "description": "<Short summary of the UX exploration>",
  "entryPoint": "/ui/fleet/p/chromeos/devices",
  "vcs": {
    "patchFile": "changes.patch",
    "baseCommitSha": "<git merge-base origin/main HEAD>"
  }
}
```

### Step 5: Preview Locally or Upload to a Static Host
Verify that `dist_proto/` contains all required files before hosting:
- `dist_proto/index.html` and `dist_proto/assets/*`
- `dist_proto/settings.js` and `dist_proto/ui_version.js`
- `dist_proto/changes.patch`
- `dist_proto/.prototype-meta.json`

Preview locally:
```bash
npx serve dist_proto
```
Then upload the `dist_proto/` directory to your static host (or, for Googlers, publish via [`http://go/fcon-labs`](http://go/fcon-labs)).

---

## UX Guardrails for Prototypes
- **Use Native Navigation**: Do not inject floating prototype bottom navbars; use the native Fleet Console sidebar and header.
- **Follow UX & Writing Guidelines**: Adhere to [writing-and-ux-principles](../writing-and-ux-principles/SKILL.md) and [high-density-ui](../high-density-ui/SKILL.md).

## Related Skills
- [ux-prototyping](../ux-prototyping/SKILL.md)
- [deploy-ui-demo](../deploy-ui-demo/SKILL.md)
- [high-density-ui](../high-density-ui/SKILL.md)
