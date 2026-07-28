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
});
```

## Environment Access
- **Localhost and Dev**: The feature flag toggle button (`<AvailableFlags />`) in the header renders only when `hostname` is `localhost` or includes `-dev`.
- **Production**: The header button is hidden to prevent end users from toggling active rollout features.

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
