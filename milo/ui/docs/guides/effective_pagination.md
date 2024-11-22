# Effective pagination

There are some modules that can help you implement a paginated view in LUCI UI.

## Simple page token based pages.

You can use the `@/common/components/param_pager` module.

It manages the following automatically:
 * page token/size updates.
 * synchronization between page token/size and URL search params.
 * (in-memory) prev page token tracking.

See [here](../../src/common/components/params_pager/doc.md) for details.

## Infinite scrolling with page token

You can use the decorated method `client.RPCMethod.queryPaged`.

It manages the query page token population automatically for you.

See [here](./make_prpc_queries.md#make-a-paginated-prpc-request-with-react_query) for details.

## Infinite scrolling with offset

You can use the module `@/generic_libs/hooks/virtualized_query`.

It lets you virtualize a query similar to how you can virtualize a list of components.

See [here](./make_prpc_queries.md#manage-queries-for-a-virtualized-list) for details.
