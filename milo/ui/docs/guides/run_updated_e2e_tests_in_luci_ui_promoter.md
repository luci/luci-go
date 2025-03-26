# Run Updated E2E Tests in LUCI UI Promoter builder

## TLDR

Update [the output_bundle_salt](../../output_bundle_salt.md).

## Explanation

[LUCI UI Promoter](https://ci.chromium.org/ui/builder-search?q=luci-ui-promoter)
runs the integration tests to ensure the staging LUCI UI build is healthy before
promoting it to production.

It checks out the code at the version that was used to build the staging version
LUCI UI when running integration tests. This prevents newly added integration
tests from stopping the release of a previous LUCI UI build.

However, a failing integration test can also mean that the test, rather than the
UI, is broken. Fixing the integration tests alone will not unblock the release
of the current staging LUCI UI. Without UI code change, LUCI UI Promoter will
keep running integration tests at an old commit.

To address this issue, you can the modify UI code slightly to trigger a new build. The
easiest way to do this is by modifying
[the output_bundle_salt](../../output_bundle_salt.md).

## Alternative Designs

### Include a hash of the integration tests in the output bundle

Pros:

* No manual update to the output bundle salt is required.

Cons:

* A new release will be triggered every time we update the integration tests

### Checkout the source code at latest commit with an identical output bundle

Pros:

* No manual update to the output bundle salt is required.
* No additional release required.

Cons:

* Difficult to implement.
  * Need to resolve the latest commit with an identical output bundle.
  * Need to tell luci-ui-promoter to re-run when integration tests are updated.
