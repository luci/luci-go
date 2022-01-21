This directory contains end to end tests of LUCI CV.

These tests are designed to run locally with fakes instead of Gerrit, Datastore,
Cloud Task Queue.


## How to run?

The usual Go way, e.g. `go test ./...` or `go test go.chromium.org/luci/cv/tests/e2e/...`

## How to debug?

Select failing test, e.g. `TestXYZ`. Run just this test in verbose mode,
which emits copious amounts of logging, as if you are looking at production CV
app, except log lines not attributed nor grouped to individual task queue
executions.

`go test -v -run TestXYZ`

To make sense of logs, it may be easiest to save them to a file:

`go test -v -run TestXYZ 2>&1 >out.log`

If test seems to hang, save your time:

`go test -v -run TestXYZ -test.timeout=1s 2>&1 >out.log`

If test is flaky, try:

`go test -v -run TestXYZ -test.count=1000 -test.failfast 2>&1 >out.log`


## Guidance on writing new tests

  * Actual tests should be added into `..._test.go` files, as usual.
  * Prefer re-using existing project config setups, instead of generating new
    ones.
  * Mark each `TestXYZ` func with `t.Parallel()`.
      * Corollary: in rare cases where your test really must not be run in
        parallel, create a dedicated sub-package for just your test and change
        global vars as necessary. `e2e` package is intentionally exportable.
  * Helper functions should be added to `e2e.go`. They should be public (see
    above).
  * Use exactly 1 Convey block inside each `TestXYZ` function.
  * Avoid deeply nested Convey blocks. Ideally there is 1 top level only.
      * Rationale: easier to debug, since each section may produce substantial
        amount of logs, which are harder to attribute to individual sub-blocks.
      * Corollary: moderate copy-pasta is fine.
  * Tests should be fast, finishing with 1s even on under-powered laptops.

## Advanced: run with flaky Datastore:

Use special `-cv.dsflakiness` flag, e.g. `go test ./... -cv.dsflakiness=0.06`.
See `flakifyDS` function for explanation of what `0.06` value actually means and
the shortcomings of the simulation.
