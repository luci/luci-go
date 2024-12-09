# Using Feature Flags

## Description

LUCI UI has a feature toggling ability that allows you to rollout
UI changes gradually to users. This feature is runs in the UI and
does not have backend support, which introduces some limitations:

1. Rollout percentages represent each individual user's
   probability of having the feature turned on/off not the total
   percentage of users who will get the feature.

2. Granualar user targeting or targeting users in a specific group is not possible.

3. Increasing the rollout percentage requires a new CL.

4. User overrides are stored in `localStorage`, which means that features won't work the same
   across devices or browsers.

## Creating a feature flag

### Step 1: create the `FeatureFlag` object

* The first step is to create a `FeatureFlag` object, this object can
  be created using the `createFeatureFlag` function.

* Pass a `FeatureFlagConfig` object to this function:

  ```ts
  const newFlag = createFeatureFlag({
    description: 'Test flag',
    namespace: 'flagKey',
    name: 'test-flag',
    percentage: 20,
    trackingBug: '123455',
  });
  ```
  for more information about this function please check [feature flags context](../../src/common/feature_flags/context.ts).

* **Recommended**: centralize the feature flag creation function call in a single file and reuse the object.
  Otherwise, your feature might behave differently in different components if the configs don't match.

* **Note**: User overrides will always override rollout percentages.

### Step 2: use the flag with `useFeatureFlag` hook

* Once you have created the `FeatureFlag` object you can then pass that to `useFeatureFlag` hook.

* This hook will calcualte the user's activation threshold, which the percentage that will activate
  the feature for the user. It will return a boolean to indicate the flag status.

* If the user has overriden the flag activation, then the hook will use instead.

Example usage:

```ts
function MyComponent() {
  const myFlagValue = useFeatureFlag(newFlag);
  return <>{myFlagValue ? 'flag is on' : 'flag is off'}</>;
}
```

For more information about this hook please read [feature flags context](../../src/common/feature_flags/context.ts).

## Overriding flag values

* Users can override flag values using the feature flags dialog,
 this can be accessed using the lab icon button in the app bar.

* User overrides will always take precendence over flag calcuated rollout percentages and values.
