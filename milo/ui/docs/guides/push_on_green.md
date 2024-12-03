# Push-on-Green

Self link: [go/luci-ui-push-on-green](http://go/luci-ui-push-on-green)

 * Design Doc: [go/luci-ui-push-on-green-mvp-dd](http://go/luci-ui-push-on-green-mvp-dd).
 * [luci-ui-promoter recipe](https://chromium.googlesource.com/infra/infra/+/main/recipes/recipes/luci_ui_promoter.py).
 * [luci-ui-promoter configuration](https://chrome-internal.googlesource.com/infra/infra_internal/+/main/infra/config/subprojects/luci_ui.star).

## Basic info
 * Push-on-green only run [from Monday to Thursday (UTC)](https://chrome-internal.googlesource.com/infra/infra_internal/+/main/infra/config/subprojects/luci_ui.star).
 * On average, the latency between landing a feature CL and landing the
   automatically created release CL is ~10 mins. This will increase as more and
   more integration tests are added.

## Common tasks
### Pause push-on-green
#### Option 1: pause the luci-ui-promoter builder
You can pause the luci-ui-promoter builder itself
[here](https://luci-scheduler.appspot.com/jobs/infra-internal/luci-ui-promoter).

Or pause the trigger for the luci-ui-promoter builder
[here](https://luci-scheduler.appspot.com/jobs/infra-internal/luci-milo-channels-poller).

Please provide a reason when pausing the builder.

#### Option 2: add a broken integration test
You can also add an integration test that always fails.

This works well when many LUCI UI sub-projects want to block the release at the
same time for different reasons.

### Revert a change
#### Option 1: revert the change CL.
Then the revert will be released to prod after the next luci-ui-promoter run.

This should be the preferred method. But it:
 * Does not work when luci-ui-promoter is paused.
    * You can solve this by manually triggering a luci-ui-promoter run via the
      [LUCI Scheduler UI](https://luci-scheduler.appspot.com/jobs/infra-internal/luci-ui-promoter).
 * Can take a while for the change to be pushed to production.

#### Option 2: revert the release CL.
Just like how you would revert any other LUCI service releases.

But you need to also pause the luci-ui-promoter builder. Otherwise, the revert
will be overridden in the next luci-ui-promoter run.

## How it works? - Life-cycle of a LUCI UI CL.

For simplicity, a lot of details are omitted. If you need more details, you can
look into how each builder is configured.

1. The CL is landed to the main branch.
2. The [luci-go-gae-tarballs-continuous builder](https://ci.chromium.org/ui/p/infra-internal/builders/prod/luci-go-gae-tarballs-continuous)
   detects a CL has landed in [luci-go](https://chromium.googlesource.com/infra/luci/luci-go/),
   and triggers a new build.
3. A new release is built (as a tarball) and uploaded.
4. LUCI UI's [channels.json](https://chrome-internal.googlesource.com/infradata/gae/+/main/apps/luci-milo/channels.json)
   is updated to use the new tarball as the staging version.
5. The [luci-ui-promoter builder](https://ci.chromium.org/ui/p/infra-internal/builders/prod/luci-ui-promoter)
   detects there's an update to LUCI UI's [channels.json](https://chrome-internal.googlesource.com/infradata/gae/+/main/apps/luci-milo/channels.json),
   triggers a new build.
6. luci-ui-promoter checks downloads the staging LUCI UI tarball, also checkouts
   the source-code used to build that tarball, and run the integration tests
   against the staging build tarball.
7. If the integration tests pass, the canary and stable channel in
   [channels.json](https://chrome-internal.googlesource.com/infradata/gae/+/main/apps/luci-milo/channels.json)
   is updated to match the staging version.
8. The [gae-deploy builder](https://ci.chromium.org/ui/p/infradata-gae/builders/ci/gae-deploy)
   detects a change in `channels.json` and deploys the new version to
   production.
