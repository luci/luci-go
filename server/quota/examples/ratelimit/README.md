# Quota Library Rate Limit Example

This example shows how to integrate the quota library into a service which uses
the LUCI server framework (but note that the only hard requirement is that the
`redisconn` library has been configured with a redis service.

## Usage

First, run `redis-server` locally (or... somewhere at least). It should be
version 5.1 at minimum, though later versions _should_ work.

Next, run this example server, pointing it at your redis service.
```shell
$ go run . -redis-addr localhost:6379 &
<observe logs, wait for the server to start>
```

This example creates a very simply policy (Default 10, Limit 60, refill 10 every
12 seconds), and only maintains a single account.

You can exercise the quota by hitting the `/global-rate-limit-endpoint` on this
local server, e.g.

```shell
$ curl http://localhost:8800/global-rate-limit-endpoint
<observe logs, expected 200 OK>
$ curl http://localhost:8800/global-rate-limit-endpoint
<observe logs, expected 429 Rate Limit Exceeded>
```

You can also reset the quota to the default (10) by hitting the
`/global-rate-limit-reset` endpoint.
