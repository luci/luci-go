# Gemini Code Assist Configuration for LUCI Milo UI

This document provides guidance to Gemini Code Assist (including Agent Mode and
`gemini-cli`) to help it generate more accurate and idiomatic code for the LUCI
Milo UI project.

## Project Overview

**LUCI Milo** is the primary user interface for the
[LUCI](https://chromium.googlesource.com/infra/luci/luci-go/+/HEAD/README.md)
project. It serves as a unified frontend for various LUCI backend services,
displaying information about builds, tests, builders, and providing analysis
tools.

The project is a monorepo containing:

- A **Go backend** that serves the UI and proxies requests to other LUCI
  services.
- A **TypeScript/React frontend** which constitutes the bulk of the user-facing
  application.

## Core Technologies

### Frontend

- **Framework:** React
- **Language:** TypeScript
- **State Management & Data Fetching:**
  [React Query (TanStack Query)](https://tanstack.com/query/latest) is the
  primary tool for managing server state.
- **Component Library:** [Material UI (MUI)](https://mui.com/)
- **Styling:** [Emotion](https://emotion.sh/docs/introduction)
  (`@emotion/styled`)
- **RPC:** Custom pRPC client implementation that integrates with React Query.
- **Legacy Code:** Some parts of the UI still use Lit and MobX. New code should
  use React and React Query. The goal is to migrate away from Lit and MobX.

### Backend

- **Language:** Go
- **RPC:** The backend uses pRPC to communicate with other LUCI services like
  Buildbucket, ResultDB, and LUCI Analysis.

## Key Concepts & Terminology

- **pRPC:** The RPC framework used for communication between the frontend, the
  Milo backend, and other LUCI services.
- **React Query:** Used extensively for fetching, caching, and updating data
  from pRPC services.
- **`usePrpcServiceClient`:** A crucial custom hook found in
  `@/common/hooks/prpc_query`. It is the standard way to get a pRPC client
  instance for use with React Query.
- **Batched Clients:** To optimize performance, some pRPC clients (e.g., for
  `buildbucket.v2.Builds`) are "batched" clients. They automatically group
  multiple individual requests from different components into a single batch RPC
  call. This is transparent to the component making the call. See
  `docs/guides/make_prpc_queries.md` for details.
- **Virtualized Queries:** The custom hook `useVirtualizedQuery`
  (`@/generic_libs/hooks/virtualized_query`) is used for efficiently fetching
  data for virtualized (infinite scrolling) lists, especially those using
  offset-based pagination.
- **LUCI Services:** Milo is a client to many other LUCI services.
  - **Buildbucket:** Manages builds and builders.
  - **ResultDB:** Stores detailed test results and artifacts.
  - **LUCI Analysis:** Analyzes test failures to find culprits and patterns.
  - **Swarming:** Manages the fleet of bots that run tests.

## Development Environment & Workflow

### Build & Dev Server (Vite & Make)

- The frontend is built using **Vite**. The configuration in `vite.config.ts`
  defines important settings like path aliases (e.g., `@/` -> `src/`).
- **Makefiles** orchestrate common development tasks. Key commands include:
  - `make presubmit`: Run all presubmit checks, including linting and tests.
    This should be run before uploading a change.
  - `make test`: Run all frontend tests.
  - `make run`: Start the local Vite development server.
  - `make build`: Create a production build of the UI assets.
  - `make protos`: Generate TypeScript bindings from protobuf definitions.

### Deployment (Google App Engine)

- The Milo UI is a set of static assets (JS, CSS, HTML) served by a **Go
  backend** running on **Google App Engine (GAE)**.
- The Go backend uses the `gaemiddleware` package and is configured via
  `app.yaml` and `cron.yaml`.
- To force a new build and deployment on CI without making code changes (e.g.,
  to re-run E2E tests with a new dependency), modify the version number in
  `output_bundle_salt.md`.

### Release Process

- User-facing changes are announced through `RELEASE_NOTES.md`.
- This file has a special format. To add a new release note:
  1. Add a description of the feature to the top "unreleased" section of the
     file.
  2. To publish the announcement, add a new release tag (e.g.,
     `<!-- __RELEASE__: <number> -->`) above your changes. The number must be
     higher than the previous release number.
  3. Once the change is deployed, the UI will show a notification to users who
     haven't seen that release number yet.

## Coding Conventions & Best Practices

### Making pRPC Queries

This is a core pattern in the codebase. Adhere to the guide in
`docs/guides/make_prpc_queries.md`.

1. **Get a client:** Use the `usePrpcServiceClient` hook. Often, a
   domain-specific hook already exists (e.g., `useBuildsClient`).
2. **Construct the query:** Use the `.query()` or `.queryPaged()` helpers on the
   client method. These helpers return a React Query options object with
   `queryKey` and `queryFn` pre-populated.
3. **Use `useQuery` or `useInfiniteQuery`:** Pass the generated options object
   to the appropriate React Query hook.
4. **Use `.fromPartial()`:** When creating request objects, use the
   `.fromPartial()` method generated by `ts-proto` to avoid type errors when the
   proto definition changes.
5. **Type Infusion:** Use the `select` option in `useQuery` to cast the response
   data to a more specific type if you know more about the data's shape than the
   proto definition (e.g., a field is guaranteed to exist because of a field
   mask).

**Example:**

```typescript
import { useQuery } from '@tanstack/react-query';
import { useBuildsClient } from '@/build/hooks/prpc_clients';
import { GetBuildRequest } from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';

function MyBuildComponent({ buildId }: { buildId: string }) {
  const client = useBuildsClient();
  const { data: build } = useQuery({
    ...client.GetBuild.query(GetBuildRequest.fromPartial({
      id: buildId,
      mask: {
        fields: ['id', 'builder', 'status', 'summaryMarkdown'],
      },
    })),
    // You can add more react-query options here.
    staleTime: 5 * 60 * 1000,
  });

  // ... render component
}
```

### Pagination

Follow the guidance in `docs/guides/effective_pagination.md`.

- For simple token-based pagination, use `@/common/components/param_pager`.
- For infinite scrolling with pRPC, use the `client.RPCMethod.queryPaged` helper
  with `useInfiniteQuery`.
- For infinite scrolling with offset-based data sources, use the
  `useVirtualizedQuery` hook.

### Styling

- Use Emotion (`@emotion/styled`) for styling components.
- For testing styles, use `@emotion/jest` and the `toHaveStyleRule` matcher.

### Dependency Management

The project uses a `package.json.md` file to document why certain dependencies
cannot be upgraded to the next major version. Before suggesting a dependency
upgrade, consult this file.

### Important Documentation

The `docs/guides` directory contains critical information about project-specific
patterns. Before answering a question, review these guides, especially:

- `docs/guides/make_prpc_queries.md`
- `docs/guides/effective_pagination.md`
- `docs/guides/make_non_overlapping_sticky_elements.md`

### Directory Structure

- `src/`: Contains all frontend source code.
- `src/proto/`: Generated TypeScript bindings for protobufs.
- `src/common/`: Shared components, hooks, and utilities.
- `src/generic_libs/`: Reusable libraries that are not specific to LUCI.
- `src/<domain>/`: Code specific to a business domain, e.g., `build`,
  `test_verdict`.
- `docs/`: Developer documentation.
