# Fleet Console Feature Flags

This document describes how feature flags work in Fleet Console (`src/fleet`).

## Centralized Registration
Register feature flags in [features.ts](../../features.ts):

```typescript
import { createFeatureFlag } from '@/common/feature_flags';

export const myNewFeatureFlag = createFeatureFlag({
  description: 'My new feature flag',
  namespace: 'fleet-console',
  name: 'my-new-feature',
  percentage: 0,
  trackingBug: '123456789',
  allowedEnvironments: ['dev'], // Defaults to ['dev']. Set to ['dev', 'prod'] to allow in production.
});
```

## Environment Filtering & UI Visibility
The feature flag toggle button (`<AvailableFlags />`) in the header automatically filters flags by the current environment (e.g. `dev` vs `prod`).
- Flags with `allowedEnvironments: ['dev']` (the default) will only be available in `localhost` and `-dev` environments.
- Flags configured with `allowedEnvironments: ['dev', 'prod']` will also be available for toggling in production.
- If no flags are allowed in the current environment, the `<AvailableFlags />` icon is hidden automatically.

## Manual Overrides in Production
You can toggle flags in production using DevTools Console:
1. Turn flag ON:
   ```javascript
   localStorage.setItem('featureFlag:fleet-console:[flag-name]', 'on');
   ```
2. Turn flag OFF / revert to default:
   ```javascript
   localStorage.removeItem('featureFlag:fleet-console:[flag-name]');
   ```
3. Refresh the page to apply changes.

## Launching Features & Cleanup
When launching a feature to all users:
1. **Do not** set `percentage: 100` or `80`. Setting `percentage >= 80` evaluates to `true` for all users and triggers console warnings.
2. **Remove the flag**: Delete the flag definition from `features.ts` and remove `useFeatureFlag` conditionals from component code so the feature path runs by default.
