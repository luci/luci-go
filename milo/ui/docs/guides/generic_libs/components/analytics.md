# Google Analytics

A set of components to integrate Google Analytics (GA) into the application,
supporting page view tracking, content grouping, and search param tracking with
PII protection.

## TrackLeafRoutePageView

Tracks a page view event when the component is mounted or when specific search
params change.

**IMPORTANT**: This should only be used in **leaf routes**. Nested trackers will
throw errors in development and warn in production.

### Usage

```tsx
import {
  TrackLeafRoutePageView,
} from '@/generic_libs/components/google_analytics';

export function MyPage() {
  return (
    <TrackLeafRoutePageView
      contentGroup="my-page-group"
      searchParamKeys={[
        'param1', // Track changes to 'param1' as new page views
        'param2',
      ]}
    >
      <MyPageContent />
    </TrackLeafRoutePageView>
  );
}
```

### Features

*   **Content Grouping**: Allows grouping pages for aggregated analysis in GA.
*   **Search Param Tracking**: Explicitly opt-in to track specific search params.
    This prevents PII leakage and excessive event generation.
*   **Context Support**: Works with `ContentGroup` and `TrackSearchParamKeys`
    from parents to allow flexible tracking configuration.

---

## Configuration components

### ContentGroup

Overrides or sets the content group for a subtree. Useful when reusing a
component in different contexts.

```tsx
<ContentGroup group="parent-page-context">
  <MyReusableComponent />
</ContentGroup>
```

### TrackSearchParamKeys

Adds keys to the list of tracked search params for the subtree. Useful when a
child component handles some params but the parent also wants to track others.

```tsx
<TrackSearchParamKeys keys={['parent-param']}>
  <MyReusableComponent />
</TrackSearchParamKeys>
```

## Internal workings

The components use React Context to aggregate configuration (content group,
tracked keys) down the tree. `TrackLeafRoutePageView` then effects the actual GA
call.
