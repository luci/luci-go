# Customizing Layouts via Route Handles

LUCI UI allows sub-apps (such as Fleet or Crystal Ball) to customize or entirely override their parent layouts. This is achieved using React Router's `useMatches` hook and the `handle` property in route configurations.

## How it works

When a route matches, React Router provides an array of all matching routes (from root to the deepest child). In [base_layout.tsx](file:///usr/local/google/home/yasinsonui/Source/chrome_infra/top_clean/infra/go/src/go.chromium.org/luci/milo/ui/src/common/layouts/base_layout.tsx), we extract the `handle` property of these matching routes:

```typescript
const matches = useMatches() as UIMatch<
  unknown,
  {
    layout?: () => ReactNode;
    defaultSidebarOpen?: boolean;
    hideGlobalFeedback?: boolean;
  }
>[];
```

The `handle` property can contain custom layout options:

- `layout`: A React component that will completely replace the `BaseLayout`.
- `defaultSidebarOpen`: A boolean to set the initial open state of the sidebar.
- `hideGlobalFeedback`: A boolean to conditionally hide the global "Submit Feedback" button in the app bar.

For properties like `defaultSidebarOpen` and `hideGlobalFeedback`, we use `.at(-1)` to ensure we pick the configuration from the deepest matched route if multiple matching routes specify it.

## Example

### Overriding layout components
A sub-app can completely replace the `BaseLayout` by exporting a `handle` with a `layout` property in its route definition:

```typescript
// src/fleet/root.ts
import { FleetLayout } from './layouts';

export const handle = {
  layout: FleetLayout,
};
```

### Toggling UI elements
A sub-app can hide elements or set their defaults by exporting properties in its route `handle`:

```typescript
// src/crystal_ball/routes.ts
export const routes = [
  {
    path: 'crystal-ball/*',
    element: <CrystalBallApp />,
    handle: {
      defaultSidebarOpen: false,
      hideGlobalFeedback: true,
    },
  },
];
```

## Why this makes LUCI UI a framework

By separating layout definition from page content and delegating layout configuration via route handles, LUCI UI functions as a framework. Sub-apps can plug into LUCI UI and configure the parent shell to best suit their design without needing to rewrite core layout shells. Sub-apps get the benefits of a shared shell while still having full control over their application's rendering.
