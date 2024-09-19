# Make pRPC Queries

Self link: [go/luci-ui-rpc-tutorial](http://go/luci-ui-rpc-tutorial)

`usePrpcServiceClient` from `@/common/hooks/prpc_query` provides an integration
solution for making pRPC queries.

It provides the following:
 * Designed to work with code-generated pRPC client and react-query.
 * Takes care of user session management, auth token injection, cache key generation.
 * Provides support for automatic request batching.

## Prerequisite: generate proto bindings
If you want to send pRPC queries to a new service, follow
[this instruction](./add_new_prpc_service.md) to set up a new service.

## Basic code examples
### Declare a client (optional).
Add a hook to construct the client.
This is typically placed in a `@/<domain>/hooks/prpc_clients.ts` package.
`<domain>` is typically a related business domain (e.g. `builds` for
`useBuildsClient`), or just `common`.

Constructing the client in a single place makes it less likely to
provide a wrong host to the client implementation. The exact placement of
the client construction hook does not matter a lot. It only affects code-
splitting.

```typescript
function useBuildsClient() {
  return usePrpcServiceClient({
    host: SETTINGS.buildbucket.host,
    ClientImpl: BuildsClientImpl,
  });
}
```

### Make a pRPC request without react-query.
The client constructed by `usePrpcServiceClient` can be used as a regular
client without react-query. e.g. `client.SearchBuilds(...)` will work as
expected.

When making requests, it's recommended that you use `.fromPartial` to construct
the request message. Without it, your code may run into type-check issue when a
new proto field is added to the request message. However, using it means you are
slightly more likely to miss required fields due to typo. Regardless, whether
you use `.fromPartial` or not has very little impact on the behavior during run
time. Check the documentation for `fromPartial` from
[ts-proto](https://github.com/stephenh/ts-proto) for more details.

```typescript
function MyComponent() {
  const client = useBuildsClient();

  client.SearchBuilds(
    SearchBuildsRequest.fromPartial({
      // Specify query parameters here.
      /* ... */
    }),
  );

  /* ... */
}
```

### Make a pRPC request with react-query.
`usePrpcServiceClient` a client decorated with methods that helps with
react-query integration. The `client.<Method>.query` helper method takes the
same parameters as the function it attaches to, and returns a ReactQuery option
with `queryKey` and `queryFn` populated.

The `queryKey` includes:
 * the user's identity.
 * the host of the service.
 * the name of the service.
 * additional headers being passed to the service.
 * the name of the RPC method.
 * the serialized request object.

Note that auth token is not included in the `queryKey`. i.e. Refreshing auth
token will not cause the query to be invalidated as long as the user's identity
is unchanged.

```typescript
function MyComponent() {
  const client = useBuildsClient();

  const regularQuery = useQuery(
    client.SearchBuilds.query(
      SearchBuildsRequest.fromPartial({
        /* ... */
      }),
    ),
  );

  /* ... */
}
```

### Customize the pRPC react-query.
Because `.query` simply returns a ReactQuery option, you can easily add more
options or override options using the object spread syntax
(`{...defaultValues, props: 'customized-value'}`). Check
[the react-query doc](https://tanstack.com/query/v4/docs/framework/react/reference/useQuery)
for more details.

```typescript
function MyComponent() {
  const client = useBuildsClient();

  const customizedQuery = useQuery({
    ...client.SearchBuilds.query(
      SearchBuildsRequest.fromPartial({
        /* ... */
      }),
    ),
    staleTime: 5000,
    /* ... */
  });

  /* ... */
}
```

### Infuse the response data type with business knowledge.
When you use `client.<Method>.query`, the type of `query.data` is inferred from
the return type of the RPC method. However, we may know more about the type of
the response than what's defined in a proto file.

For example, we may know that a build we queried from buildbucket belongs to a
builder and the `build.builder` is always defined as long as the field mask
specified in the request. In such case, it's usually a good idea to cast the
return type in the `select` function so the type casting is located close to
where the data is sourced and does not need to be repeated in many places.

```tsx
// Type definitions like this is typically placed in `@/<domain>/types` if it's
// reusable.
interface OutputBuild extends Build {
  // In the generated binding, `Build.builder` is nullable. But we know it
  // cannot be `null`/`undefined` as long as `builder` is not excluded by the
  // field mask.
  readonly builder: BuilderID;
}

function MyComponent() {
  const client = useBuildsClient();

  const {data, isLoading, isError, error} = useQuery({
    ...client.GetBuild.query(
      GetBuildRequest.fromPartial({
        mask: {
          fields: ["builder", /* ... */],
        },
        /* ... */
      }),
    ),
    select: (data) => data as OutputBuild,
  });
  if (isError) {
    throw error;
  }
  if (isLoading) {
    return <></>;
  }

  // We can use `data.builder.project` without using non-null coercion
  // (e.g. `data.builder!.project` ) in the rest of the codebase.
  console.log(data.builder.project);

  /* ... */
}
```

### Make a paginated pRPC request with react-query.
In addition to `queryKey` and `queryFn`, `.queryPaged` also populates the
`getNextPageParam`. It assumes the RPC uses the AIP-158 pagination. i.e. The
request message must take a `pageToken` field while the response message must
have a `nextPageToken` field. This makes it suitable to be used with
`useInfiniteQuery`.

Note that you does not necessarily need to use `useInfiniteQuery` to implement
pagination. See
[the react-query doc](https://tanstack.com/query/v4/docs/framework/react/guides/paginated-queries)
for details.

```typescript
function MyComponent() {
  const client = useBuildsClient();

  const paginatedQuery = useInfiniteQuery(
    client.SearchBuilds.queryPaged(
      SearchBuildsRequest.fromPartial({
        /* ... */
      }),
    ),
  );

  /* ... */
}
```

### Make pRPC query with additional headers.
Sometimes it's useful to pass additional gRPC metadata through headers.

Note that critical headers (`accept`, `content-type`, `authorization`) will not
be overwritten even when specified.

```typescript
function useBuildsClient(additionalHeaders: HeadersInit) {
  return usePrpcServiceClient({
    host: SETTINGS.buildbucket.host,
    ClientImpl: BuildsClientImpl,
    additionalHeaders,
  });
}
```

### Make pRPC query using ID tokens instead of access tokens.
Most LUCI services use access token to authenticate pRPC requests. In case you
need to call a services that uses the ID token, you can specify
`initUseGetAuthToken`.

```typescript
import { useGetIdToken } from '@/common/components/auth_state_provider';

function useBuildsClient() {
  return usePrpcServiceClient({
    host: SETTINGS.buildbucket.host,
    ClientImpl: BuildsClientImpl,
    initUseGetAuthToken: useGetIdToken,
  });
}
```

## Advanced code examples
### Automatically batch pRPC queries together.
In some cases you may have many individual components that need to query a
service. The service provides a batch endpoint. In theory, you can send a batch
request at the parent component and distribute the query results to the child
components. However, there are some challenges in doing so:
 * It might be difficult for the parent component to know the exact queries
   required by the child components. This is especially true when filtering and
   list virtualization are used.
 * When the list of displayed child component changes slightly (e.g. one more
   child component is displayed), a new batch query needs to be sent. We will
   not be able to reuse the cache of the previous batch query even though the
   two batch queries largely overlaps.
 * Having the parent component query everything may make the parent component
   tightly couple with the child component.

To overcome these challenges, you can use a client implementation that can
automatically batch requests together (see `@/proto_utils/batched_clients/*`).

For example, [the builder list page](https://ci.chromium.org/ui/p/chromium/builders)
renders a filterable, virtualized list of builders. Each builder row, when
rendered, needs to query builds that belong to this builder from buildbucket
using the `buildbucket.v2.Builds.SearchBuilds` RPC. This may lead to an
explosion of queries when the page is loaded. Ideally, we want to use the
`buildbucket.v2.Builds.Batch` RPC to batch RPC requests together. However,
deciding which builders we can query on the parent component is difficult
because we don't know which builder is rendered by the filterable, virtualized
builder list. Also, when user scrolls the page, we want to query builds for the
newly visible builders without discarding the already queried builds for the
already visible builders. To address this issue, we can use
`BatchedBuildsClientImpl`. It can automatically batch
`buildbucket.v2.Builds.SearchBuilds` requests together.

Here's a code snippet to illustrate how you can use a batched client
implementation. For the most part, you can just use it as if it's a regular
client and everything will work as expected. See
`@/build/components/builder_table` if you want a more concert example.

```typescript
function useBatchedBuildsClient(maxBatchSize = 200) {
  return usePrpcServiceClient(
    {
      host: SETTINGS.buildbucket.host,
      ClientImpl: BatchedBuildsClientImpl,
    },
    { maxBatchSize },
  );
}

interface BuilderRowProps {
  readonly builder: BuilderID;
}

function BuilderRow({builder}: BuilderRowProps) {
  const client = useBatchedBuildsClient();

  const justLikeARegularQuery = useQuery(
    client.SearchBuilds.query(
      SearchBuildsRequest.fromPartial({
        builder,
        /* ... */
      }),
    ),
  );

  /* ... */
}
```

#### Why my requests are not batched together?
This can due to a number of reasons. It usually depends on the batch client
implementation. The following sections assumes a typical,
`@/generic_libs/tools/batched_fn` based batch client implementation is used.

##### The batch `ClientImpl` decided that the requests should not be batches together.
This typically happens when
1. the request size is too large (e.g.
`buildbucket.v2.Builds.Batch` allows up to 200 requests to be batched together).
2. the requests are not eligible for batching (e.g.
`luci.resultdb.v1.ResultDB.BatchGetTestVariants` can only query data from one
invocation at a time).

##### The requests are not made in time for batching.
A batch client cannot wait indefinitely to collect requests. To reduce query
latency, usually a batch client will only wait a very short period of time
before sending out the requests. By default, a batch client will send out
the requests in the next
[JS event cycle](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Event_loop#event_loop).
This means, by default, you won't be able to batch requests from multiple React
rendering cycles together. To address this issue, you can extend the maximum
pending duration of a request.

```typescript
function MyComponent() {
  const justLikeARegularQueryWithAdditionalLatency = useQuery(
    client.SearchBuilds.query(
      SearchBuildsRequest.fromPartial({
        /* ... */
      }),
      // Requests made in the next 100ms can be batched together with this
      // request.
      { maxPendingMs: 100 },
    ),
  );
}
```

##### The RPC requests were sent to different batch `ClientImpl` object instances.
The batch `ClientImpl` usually needs to use an internal state to keep track of
the list of queries made by the caller. Therefore, it usually can only batch
calls made to the same `ClientImpl` object instance.

`usePrpcServiceClient` keeps track of a global cache for all the client object
instances. This allows it to return the same instance of `ClientImpl` when a
`ClientImpl` is requested by another component, or in another rendering cycle.
The client object instance is keyed by the parameters used to construct it. For
example `useBatchedBuildsClient(200)` and `useBatchedBuildsClient(100)` will
return two different object instances of batched builds client, because they
have different batch size. Calls to the client with batch size 200 will never be
batched with calls to the client with batch size 100. On the other hand,
multiple `useBatchedBuildsClient(200)` will return the same object instance.
Calls made to the client will be eligible for batching.

In the following example, queries from `<ComponentA />` and `<ComponentC />`
will be batched together because they use the same client object instance.
Queries from `<ComponentB />` will not be batched with queries from
`<ComponentA />` and `<ComponentC />` because `<ComponentB />` uses a different
client object instance due to it using a different set of parameters to
construct the client.

```typescript
function useBatchedBuildsClient(maxBatchSize = 200) {
  return usePrpcServiceClient(
    {
      host: SETTINGS.buildbucket.host,
      ClientImpl: BatchedBuildsClientImpl,
    },
    // Note that `{ maxBatchSize }` is serialized into a string when it's used
    // as a key to retrieve the `BatchedBuildsClientImpl` client instance.
    // So even though a new `{ maxBatchSize }` instance is created in each
    // `useBatchedBuildsClient` call, the same instance of
    // `BatchedBuildsClientImpl` will be returned as long as the `maxBatchSize`
    // is the same.
    { maxBatchSize },
  );
}

function ComponentA() {
  // The same client instance as <ComponentC />.
  const client = useBatchedBuildsClient(200);
  const query = useQuery(
    client.SearchBuilds.query(
      SearchBuildsRequest.fromPartial({
        /* ... */
      }),
    ),
  );

  /* ... */
}

function ComponentB() {
  // A different client instance because a different batch size is used.
  const client = useBatchedBuildsClient(100);
  const query = useQuery(
    client.SearchBuilds.query(
      SearchBuildsRequest.fromPartial({
        /* ... */
      }),
    ),
  );

  /* ... */
}

function ComponentC() {
  // The same client instance as <ComponentA />.
  const client = useBatchedBuildsClient(200);
  const query = useQuery(
    client.SearchBuilds.query(
      SearchBuildsRequest.fromPartial({
        /* ... */
      }),
    ),
  );

  /* ... */
}
```

### Manage queries for a virtualized list.
This is not specific to pRPC queries.

If you are
 * building a [virtualized](https://www.kirupa.com/hodgepodge/ui_virtualization.htm)
   list, and
 * the RPC backing the list uses an offset to query a list of items.

Then `@/generic_libs/hooks/virtualized_query` can help you manage queries
efficiently.

`@/generic_libs/hooks/virtualized_query` works by partitioning a query into
chunks.
 * Each individual chunk can be queried from the service without hitting a
   page size limit.
 * Only query the chunks that are currently required (i.e. visible on the
   screen).
 * The chunks are queried in parallel to reduce query latency.
 * Each chunk can be cached separately so invalidating one chunk does not
   invalidate other chunks.
 * The chunks are aligned. Updating the item index domain will only cause some
   chunks to be updated. (e.g. index `[2, 16)` is divided into `[2, 5)`,
   `[5, 10)`,  `[10, 15)`, `[15, 16)`. Updating the index domain to `[1, 17)`
   will result in chunks `[1, 5)`, `[5, 10)`, `[10, 15)`, `[15, 17)`. Only the
   first and last chunk requires an update. All the chunks in-between can reuse
   their query cache).

`@/generic_libs/hooks/virtualized_query` only handles query partitioning and
active partition selection. The actual query is entirely managed by
`react-query`. Therefore, all react-query cache management/error handling
primitives can be used.

Here's a code snippet to illustrate how you can use the
`@/generic_libs/hooks/virtualized_query`. See
`@/test_verdict/components/blamelist_table` if you want a more concert example.

```tsx
function MyComponent() {
  const maxIndex = 200;
  const initialTopMostItemIndex = 130;

  const virtualizedQueries = useVirtualizedQuery({
    // Limit the query index range to `[0, maxIndex)`. Negative ranges
    // are supported (e.g. `[-100, -50)`).
    //
    // This is useful to avoid querying items from undefined domains. For
    // example, querying a commit with a commit position greater than the commit
    // position of tip-of-branch commit will lead to 404.
    rangeBoundary: [0, maxIndex],
    // No more than 50 items will be queried at a time.
    //
    // This is useful to limit the page size so we don't exceed the maximum
    // supported page size by the RPC.
    interval: 50,
    // The initial range to query.
    //
    // This is useful when the virtualized list is pre-scrolled to a item on
    // render instead of starting at the top.
    initRange: [initialTopMostItemIndex, initialTopMostItemIndex + 1],
    // The waiting time before sending a query for a chunk when the chunk is
    // scrolled into view.
    //
    // This is useful to avoid sending queries for chunks that the user simply
    // scroll past.
    delayMs: 100,
    // Defines how to generated a react-query option to query items from `start`
    // (inclusive) to `end` (exclusive).
    genQuery: (start, end) => ({
      // Regular react-query primitives can be used.
      queryKey: [/* ... */],
      queryFn: () => { /* ... */ },
      staleTime: Infinity,
      // IMPORTANT:
      // The query must return an array of items. The `i`th item
      // (`i ∈ [start, end)`) in the array corresponds to `start + i`.
      select: (data) => data.items,
    }),
  });

  return (
    <Virtuoso
      totalCount={maxIndex}
      initialTopMostItemIndex={initialTopMostItemIndex}
      rangeChanged={(range) => {
        // Notify `virtualizedQueries` that the visible range has changed.
        // `+ 1` because `virtualizedQueries.setRange`'s end bound is
        // exclusive while `range.endIndex` is inclusive.
        virtualizedQueries.setRange([range.startIndex, range.endIndex + 1]);
      }}
      itemContent={(index) => {
        // `virtualizedQueries.get(index)` returns a regular react-query
        // `UseQueryResult` where the item is an item at `index`.
        const {data, isLoading, isError, error} = virtualizedQueries.get(index);

        /* ... */
      }}
      {/* ... */}
    />
  );
}
```
