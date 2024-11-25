# Google Analytics Tracker

## Example
### Track page view
IMPORTANT: You should only add `TrackLeafRoutePageView` to the leaf routes.
Nested `TrackLeafRoutePageView`s will
 * throw an error in development mode.
 * be NOOP in production mode
   * it will still log a warning.

```tsx
function MyTab() {
  const [searchParams] = useSyncedSearchParams();

  const mySearchParam1 = searchParams.get('my-search-param-key-1');
  const mySearchParam2 = searchParams.get('my-search-param-key-2');
  const myFilter = searchParams.get('my-filter-key');
  const mySearchParamWithPII = searchParams.get('my-search-param-with-pii-key');

  return (
    <>
      {mySearchParam1}
      {mySearchParam2}
      {myFilter}
      {mySearchParamWithPII}
    </>
  )
}

export function Component() {
  return (
    <TrackLeafRoutePageView
      contentGroup="my-tab"
      searchParamKeys={[
        // List all the search param keys you want to track here.
        // Any change to those search param keys will trigger a new page view
        // event.
        'my-search-param-key-1',
        'my-search-param-key-2',

        // Do not add 'my-filter-key' here unless you want any change to the
        // filter to be recorded as a page view event. In most cases, you should
        // not track search params that act as filters.
        // 'my-filter-key',

        // Do not add 'my-search-param-with-pii-key' here because we must not
        // collect PII to GA.
        // 'my-search-param-with-pii-key',
      ]}
    >
      {/* The error boundary should be placed inside the tracker component.
        * This allows the tracking to work even when the page itself fails. */}
      <RecoverableErrorBoundary key="my-tab" >
        <MyTab />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  )
}
```

### Track tab view within the context of a parent page.
The same `<MyTab />` component might be used under different pages. In that
case, you may want to give different content groups to `<MyTab />`s mounted at
different places.

Let say we have a route definition like the following:
```ts
// @/my-area/routes.ts

export const myRoutes: RouteObject[] = [
  {
    path: 'page-one',
    lazy: () => import('@/my-area/pages/my-page-one'),
    children: [
      {
        path: 'my-tab',
        lazy: () => import('@/my-area/pages/my-tab'),
      },
      {
        path: 'my-other-tab',
        lazy: () => import('@/my-area/pages/my-other-tab'),
      },
    ],
  },
  {
    path: 'page-two',
    lazy: () => import('@/my-area/pages/my-page-two'),
    children: [
      {
        path: 'my-tab',
        lazy: () => import('@/my-area/pages/my-tab'),
      },
      {
        path: 'my-yet-other-tab',
        lazy: () => import('@/my-area/pages/my-yet-other-tab'),
      },
    ],
  },
];
```

We can declare different content-groups for different parent page components.
```tsx
// @/my-area/pages/my-page-one

function MyPageOne() {
  return (
    <>
      <span>Page One</span>
      <Outlet />
    </>
  )
}

export function Component() {
  return (
    // Define a content group for the parent page.
    // When user visits `<MyTab />` under `<MyPageOne />`, the content group
    // will be reported as `my-page-one | my-tab`.
    // Likewise, in `@/my-area/pages/my-page-two`, you can use the same
    // mechanism to give it a different content group (e.g. `my-page-two`).
    <ContentGroup group="my-page-one">
      <RecoverableErrorBoundary key="my-page-one">
        <MyPageOne />
      </RecoverableErrorBoundary>
    </ContentGroup>
  );
}
```

### Track search param keys used by the parent page.
To keep the components decoupled, the child tab component may not know what are
the keys used by the parent page component. However, you may still want to track
the search param keys used by the parent page component.

You can use `<TrackSearchParamKeys />` to add keys to the list of tracked
params.

```tsx
function MyPageOne() {
  const [searchParams] = useSyncedSearchParams();
  const myParentPageParam = searchParams.get('my-parent-page-param-key');

  return (
    <>
      <span>Page One: {myParentPageParam}</span>
      <Outlet />
    </>
  )
}

export function Component() {
  return (
    <TrackSearchParamKeys keys={['my-parent-page-param-key']}>
      <ContentGroup group="my-page-one">
        <RecoverableErrorBoundary key="my-page-one">
          <MyPageOne />
        </RecoverableErrorBoundary>
      </ContentGroup>
    </TrackSearchParamKeys>
  );
}
```

## Future improvements
 * Build page title PII collection prevention similar to how we build URL search
   params PII collection prevention.

## Design decisions
 * Do not track all search param keys by default.
   * Reduces the chance of collecting PII into search params.
   * Reduces the chance of generating excessive amount of page view events due
     to frequent filter updates.
 * Allow parents to tweak tracking config via `<ContentGroup />`.
   * Support reusing components in different places yet track them as separate
     content groups.
 * Allow parents to add tracking search params via `<TrackSearchParamKeys />`.
   * Allow decoupling of the parent components and their child components.
 * Do not allow nested `<TrackLeafRoutePageView />`.
   * Triggering more than one page view events per user visit can be very
     confusing.
 * Build trackers as components rather hooks.
   * Pros: This allows us to provide contexts to the child components. This
     enables the parents to add additional tracking config via
     `<ContentGroup />` and `<TrackSearchParamKeys />`.
   * Cons: If the tracking component is rendered conditionally (e.g. due to
     loading spinners), the tracking will be inconsistent. However, this is
     usually not an issue since we typically declare the trackers in a
     logic-less component.
