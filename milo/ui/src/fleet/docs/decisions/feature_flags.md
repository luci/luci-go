# Fleet Console Feature Flags

This document describes the usage of feature flags within the Fleet Console (`src/fleet`) and how they differ between environments.

For the general LUCI UI feature flags documentation, see [using_feature_flags.md](../../../../docs/guides/using_feature_flags.md).

## Centralized Definition

Feature flags for the Fleet Console are centralized in [features.ts](../../features.ts). Always register new flags here to keep them consistent and reusable across components:

```typescript
import { createFeatureFlag } from '@/common/feature_flags';

export const myNewFeatureFlag = createFeatureFlag({
  description: 'My new awesome feature flag',
  namespace: 'fleet-console',
  name: 'my-new-feature',
  percentage: 0, // Starts at 0% rollout
  trackingBug: '123456789',
});
```

## Environment Restrictions

To prevent incomplete or experimental features from appearing in production during development, the toggling UI is restricted by environment.

### 1. Localhost and Dev Environments
The experimental/flask icon button (`<AvailableFlags />`) in the [Header](../../layouts/header.tsx) app bar is **only rendered** if the current hostname is `localhost` or contains `-dev`:

```typescript
  const showAvailableFlags =
    window.location.hostname === 'localhost' ||
    window.location.hostname.includes('-dev');
```

### 2. Production Environment
In production, the flask icon button is **hidden** by default to prevent end users from toggling in-development features.

## Toggling Flags Manually in Production

Even though the header UI is hidden in production, the underlying `useFeatureFlag` hook still queries `localStorage` for overrides. You can manually override and toggle any flag using the browser Developer Tools:

1. Open the browser **Developer Tools Console** on the production site.
2. Run the following command to turn the flag **on**:
   ```javascript
   localStorage.setItem('featureFlag:fleet-console:[flag-name]', 'on');

   // e.g.: localStorage.setItem('featureFlag:fleet-console:smart-repair-tab', 'on');
   ```
3. To turn the flag **off** or revert to default, run:
   ```javascript
   localStorage.removeItem('featureFlag:fleet-console:[flag-name]');
   ```
4. **Refresh the page** for the changes to take effect.
