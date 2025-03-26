# Authentication & Authorization

This doc only covers the UI portion of the auth space.

The server flow is implemented with an encrypted-cookie, OAuth2.0 based
authentication. Check [go/milo-cookie-based-authentication](http://go/milo-cookie-based-authentication)
for details.

## Making authenticated requests

### pRPC

If you are making a pRPC request, typically you can just use the
`usePrpcServiceClient` hook. It will take care of user session management,
auth token injection, etc for you. See [here](./make_prpc_queries.md) for
details.

### Non-pRPC

If you are making a non-pRPC request, you can obtain the auth token using
the `useGetAccessToken` and the `useGetIdToken` hooks from
`@/common/components/auth_state_provider`.

Check the documentation on those hooks for details.

## Check whether user has certain permission

This will be useful if you need to render something base on the permission the
user has (e.g. a retry build button).

You should use the `usePermCheck` hook from `@/common/hooks/perm_check`. It
automatically batches all the permission checks in the same rendering cycle
together. See the documentation on that hook for details.

Note that `usePermCheck` is only able to check permissions registered
[here](https://chromium.googlesource.com/infra/luci/luci-go/+/main/milo/rpc/batch_check_permissions.go#27).
You need to register your permission there before using this hook.
