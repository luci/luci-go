# params_pager

## Overview
This package helps you build regular, page-token based, paginated views.

Requirements:
 * You must use a AIP-158 style pagination API (i.e. pageSize and pageToken
   fields).

It manages the following automatically:
 * page token/size updates.
 * synchronization between page token/size and URL search params.
 * (in-memory) prev page token tracking.

## Example
```tsx
function MyPaginatedComponent() {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [myFilter, setMyFilter] = useState();

  // Construct a pager context that holds the state and configuration for the
  // pager.
  const pagerCtx = usePagerContext({
    pageSizeOptions: [25, 50, 100, 200],
    defaultPageSize: 50,
    // You are not limited to one `usePagerContext` call.
    // But when you have multiple `usePagerContext` calls, please ensure that
    // they don't share the same `pageSizeKey` and `pageTokenKey`.
    // Otherwise, those pagers will override each other's value in the URL.
    pageSizeKey: 'limit',
    pageTokenKey: 'cursor',
  });


  // There's a list of helper functions that can help you extract data from the
  // pagerCtx.
  // See `getPageSize`, `getPageToken`, `pageSizeUpdater`, `pageTokenUpdater`.
  const pageSize = getPageSize(pagerCtx, searchParams);
  const pageToken = getPageToken(pagerCtx, searchParams);

  const {data, isLoading, isError, error} = useQuery({
    queryKey: ['my', 'query', 'keys', myFilter, pageSize, pageToken],
    queryFn: () => queryMyData(myFilter, pageSize, pageToken),
  });
  if (isError) {
    throw error;
  }

  // It's ok to render param pagers conditionally.
  if (isLoading) {
    return <div>loading...</div>;
  }

  return (
    <>
      <input
        value={myFilter}
        onChange={(e) => {
          setMyFilter(e.target.value);

          // When the filter changes, usually you want to clear out all the
          // page tokens. An AIP-158 page token is only valid for the filter
          // option that generated it.
          setSearchParams(emptyPageTokenUpdater(pagerCtx));
        }}
      />

      <MyContent data={data} />

      <ParamsPager pagerCtx={pagerCtx} nextPageToken={data.nextPageToken} />

      {/* You can add as many pagers as you like. */}
      <ParamsPager pagerCtx={pagerCtx} nextPageToken={data.nextPageToken} />

      {/* You can pass the pagerCtx around (e.g. to another component). */}
      <MyNestedComponent pagerCtx={pagerCtx} nextPageToken={data.nextPageToken}/>

      {/* Or you can build your own UI component for the pager. */}
      <MyCustomPager pagerCtx={pagerCtx} nextPageToken={data.nextPageToken} />
    </>
  );
}

```

## Design decisions
 * Hold most pager configuration in a single object (the pager context).  
   This ensures the pager component and the pager state getters/setters uses
   the same set of configuration. If different sets of configurations are
   used (e.g. different default page sizes are passed to `getPageSize` and
   `<ParamPager />`), the state may appear to be inconsistent (e.g. the
   page size used in the query is 50 but the highlighted selected page size
   is 25). Some pager configuration (e.g. `nextPageToken`) are not hold in
   the context because they may have not been initialized when pager context
   is constructed.
 * Force the pager context to be constructed via a hook.  
   This allows us to enforce that the state is always created and persisted
   when the pager is used. `eslint(react-hooks/rules-of-hooks)` will enforce
   that the state is not accidentally unloaded. If we hold the state in the
   pager component, users may unload the pager therefore accidentally discard
   the previous page tokens state when rendering a loading spinner.
 * Force all page token/size getters/setters to take a pager context.  
   This encourage users to lift the `usePagerContext` call to the top level
   component under which the pager and the state (e.g. page size, page token)
   it hold is used. As long as the pager is used in any form in the
   component, the pager state will not be discarded, even when the pager
   component itself is not rendered.
 * Prev page tokens are only kept in memory so we don't have excessively long
   URLs.
