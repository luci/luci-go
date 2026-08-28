# Using Feature Flags in LUCI Milo UI

## Overview

LUCI Milo UI provides a unified client-side feature flag system that supports gradual user rollouts, environment gating (`dev` vs. `prod`), environment-specific percentage rollouts, central pre-registration, and real-time developer overrides via the top app bar dialog.

### Key Capabilities

1. **Central Pre-Registration**: Any feature flag declared with `createFeatureFlag()` in eagerly imported configuration modules (such as `src/fleet/features.ts` or `src/common/feature_flags/`) is automatically registered in the global flag registry. It appears in the top app bar feature flags modal (`<AvailableFlags />`) for discovery and toggling upon app load. Flags declared inside lazily loaded route chunks (`React.lazy`) are registered when that JS chunk is imported.
2. **Dual Context Support**:
   - **React Components**: Use the `useFeatureFlag(flag)` hook.
   - **Non-Hook / Routing Code**: Use the `getFeatureFlagValue(flag)` synchronous helper.
3. **Environment Restrictions & Percentages**: Restrict flags to specific target environments (e.g. `allowedEnvironments: ['dev']`) or set environment-specific rollout percentages (e.g. `percentage: { dev: 100, prod: 0 }`). Omitting `allowedEnvironments` defaults to `['dev', 'prod']` (available everywhere).
4. **Local Overrides**: Developers and testers can toggle flags in the UI modal or via `localStorage.setItem('featureFlag:<namespace>:<name>', 'on' | 'off')`. Overrides always take precedence over rollout percentages.

---

## Defining a Feature Flag

Declare feature flags in a domain-specific file (e.g. `src/<domain>/features.ts`):

```ts
import { createFeatureFlag } from '@/common/feature_flags';

// Option A: Uniform percentage across environments
export const enableNewDashboard = createFeatureFlag({
  namespace: 'my-feature-area',
  name: 'new-dashboard',
  description: 'Enable the redesigned analytics dashboard.',
  percentage: 0,
  trackingBug: '123456789',
  allowedEnvironments: ['dev', 'prod'], // Optional, defaults to ['dev', 'prod']
});

// Option B: Environment-specific rollout percentages (Dev -> Prod rollout workflow)
export const enableExperimentalMetrics = createFeatureFlag({
  namespace: 'fleet-console',
  name: 'experimental-metrics',
  description: 'Enable experimental metrics display.',
  percentage: { dev: 100, prod: 0 }, // 100% in Dev, 0% in Prod
  trackingBug: '388907865',
});
```

### Configuration Options

| Option | Type | Description |
| :--- | :--- | :--- |
| `namespace` | `string` | Group namespace for the flag (e.g. `'fleet-console'`, `'test-investigation'`). |
| `name` | `string` | Unique identifier within the namespace. |
| `description` | `string` | Human-readable explanation displayed in the `<AvailableFlags />` modal. |
| `percentage` | `number \| { dev?: number, prod?: number }` | Rollout percentage threshold. Can be a single number or environment-specific map (e.g. `{ dev: 100, prod: 0 }`). Values $\ge 80\%$ trigger a cleanup warning. |
| `trackingBug` | `string` *(optional)* | Bug ID tracking the feature rollout. |
| `allowedEnvironments` | `readonly ('dev' \| 'prod')[]` *(optional)* | Allowed environments. Defaults to `['dev', 'prod']`. Flags restricted to `['dev']` evaluate to `false` in production regardless of percentage. |

---

## Evaluating Feature Flags

### 1. In React Components (`useFeatureFlag`)

```tsx
import { useFeatureFlag } from '@/common/feature_flags';
import { enableNewDashboard } from '@/my_domain/features';

export function DashboardContainer() {
  const isEnabled = useFeatureFlag(enableNewDashboard);

  if (!isEnabled) {
    return <LegacyDashboard />;
  }
  return <NewDashboard />;
}
```

### 2. Outside React Hooks / In Route Tables (`getFeatureFlagValue`)

Use `getFeatureFlagValue(flag)` when evaluating flags in route definitions, navigation builders, or non-React utilities:

```ts
import { getFeatureFlagValue } from '@/common/feature_flags';
import { enableNewDashboard } from '@/my_domain/features';

export const myRoutes = [
  {
    path: 'dashboard',
    element: getFeatureFlagValue(enableNewDashboard) ? (
      <NewDashboardRoute />
    ) : (
      <LegacyDashboardRoute />
    ),
  },
];
```

---

## Overriding Flag Values

- **App Bar Modal**: Click the lab/flask icon in the top app bar to open the Feature Flags dialog and toggle any registered flag.
- **LocalStorage API**: Set `featureFlag:<namespace>:<name>` to `'on'` or `'off'` in the browser console:
  ```js
  localStorage.setItem('featureFlag:my-feature-area:new-dashboard', 'on');
  ```
