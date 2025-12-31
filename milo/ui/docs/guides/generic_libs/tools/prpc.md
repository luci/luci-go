# pRPC tools

Utilities for interacting with pRPC services in the browser.

## PrpcClient

A robust client for making pRPC calls using the "canonical JSON" protocol over
HTTP/HTTPS.

### Usage

```ts
import { PrpcClient } from '@/generic_libs/tools/prpc_client';

const client = new PrpcClient({
  host: 'milo.luci.app',
  getAuthToken: () => getOAuthToken(),
});

const result = await client.request(
  'buildbucket.v2.Builds',
  'GetBuild',
  { id: '12345' }
);
```

### Features

*   **Auth Support**: Accepts a `getAuthToken` callback.
*   **Protocol Compliance**: Handles `X-Prpc-Grpc-Code` headers and XSSI prefix
    (`)]}'`) stripping automatically.
*   **Fetch Injection**: Can inject a custom `fetch` implementation (useful for
    testing or mocking).

## pRPC utils

(Check `src/generic_libs/tools/prpc_utils` for helper functions)
*   **`genBatchClient`**: Generates a client that automatically batches requests
    for compatible services (like Buildbucket).
*   Errors and code definitions are also available.
