# Quota Library Rate Limit Example

This example shows how to integrate the quota library into a GAE service.
Not intended to be run on App Engine. A GAE demo can be found under
[examples/appengine/quota](../../../../examples/appengine/quota).

## Usage

Startup:
```shell
$ go run .&
<observe logs, wait for the server to start>
```

Verify that rate limiting works:
```shell
$ curl http://localhost:8800/global-rate-limit-endpoint
<observe logs, expected 200 OK>
$ curl http://localhost:8800/global-rate-limit-endpoint
<observe logs, expected 429 Rate Limit Exceeded>
<wait 60 seconds>
$ curl http://localhost:8800/global-rate-limit-endpoint
<observe logs, expected 200 OK>
```

Verify that resetting quota works:
```shell
$ curl http://localhost:8800/global-rate-limit-reset
<observe logs, expected 200 OK>
$ curl http://localhost:8800/global-rate-limit-endpoint
<observe logs, expected 200 OK>
$ curl http://localhost:8800/global-rate-limit-endpoint
<observe logs, expected 429 Rate Limit Exceeded>
```

Verify that multiple resets don't stack:
```shell
$ curl http://localhost:8800/global-rate-limit-reset
<observe logs, expected 200 OK>
$ curl http://localhost:8800/global-rate-limit-reset
<observe logs, expected 200 OK>
$ curl http://localhost:8800/global-rate-limit-endpoint
<observe logs, expected 200 OK>
$ curl http://localhost:8800/global-rate-limit-endpoint
<observe logs, expected 429 Rate Limit Exceeded>
```
