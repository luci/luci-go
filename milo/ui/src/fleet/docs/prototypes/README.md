# Fleet Console Static Prototyping Guide

This guide explains how to build, package, and host standalone static Fleet Console UI prototypes using real Fleet Console React components and `FleetConsoleMockAPI`.

## Overview

A static Fleet Console prototype ("Easy Mock") runs entirely in the browser without requiring a live Go backend or App Engine deployment.

The workflow is split into two parts:
1. **Build & Package (`milo/ui/src/fleet/`)**: Compile the Fleet Console UI with client-side mock pRPC fixtures (`FleetConsoleMockAPI`) and export a self-contained static directory (`dist_proto/`).
2. **Host Anywhere**: Serve `dist_proto/` locally (`npx serve dist_proto`), upload it to any static web host, or (for Googlers) publish it to the FCon Labs gallery at [go/fcon-labs](http://goto.google.com/fcon-labs).

## Required Bundle Contents (`dist_proto/`)

To ensure every prototype can be viewed in a browser and reconstructed in source control later, a complete `dist_proto/` bundle contains:

| File / Directory | Purpose |
| :--- | :--- |
| `index.html` & `assets/*` | Compiled Vite static bundle built with relative asset paths (`npx vite build --base=./ --outDir=dist_proto`). |
| `settings.js` & `ui_version.js` | Runtime settings and client-side mock pRPC interceptor harness. |
| `changes.patch` | Complete git diff (`git diff $(git merge-base origin/main HEAD) -- src/fleet`) so another developer or agent can apply the prototype with `git apply --3way changes.patch`. |
| `.prototype-meta.json` | JSON manifest recording `name`, `title`, `description`, `entryPoint`, and `vcs.baseCommitSha`. |

## Step-by-Step Build Instructions

1. **Develop UI & Mock Fixtures**:
   Edit components under `src/fleet/` and add or customize synthetic pRPC fixtures in `src/fleet/testing_tools/mock_api/mock_api_handler.ts`.
2. **Compile Static Assets**:
   ```bash
   npx vite build --base=./ --outDir=dist_proto
   ```
3. **Export Source Patch & Metadata**:
   ```bash
   git add -N -- src/fleet
   git diff "$(git merge-base origin/main HEAD)" -- src/fleet > dist_proto/changes.patch
   ```
4. **Preview or Upload**:
   - **Local preview**: `npx serve dist_proto`
   - **External static hosting**: Upload `dist_proto/` to GitHub Pages, Firebase Hosting, Netlify, S3, or GCS.
   - **Internal gallery (Googlers)**: See [go/fcon-labs](http://goto.google.com/fcon-labs) for publishing `dist_proto/` to the shared FCon Labs directory.

## Related Documentation
- [`mock-api-architecture.md`](../decisions/mock-api-architecture.md)
- [`adr-002-e2e-prototyping-pipeline.md`](../decisions/adr-002-e2e-prototyping-pipeline.md)
- [`fcon-easy-mock` Agent Skill](../../.agents/skills/fcon-easy-mock/SKILL.md)
