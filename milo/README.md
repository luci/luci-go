# Milo - The UI for LUCI.

Milo is the user interface for LUCI. It displays information from Buildbucket
and ResultDB about builders, builds, and test results, and can be configured to
display custom consoles.

## Releasing

Releases are automatically pushed to luci-milo-dev on commit by the
[gae-deploy](https://ci.chromium.org/p/infradata-gae/builders/ci/gae-deploy)
builder.

To push to prod, the steps are:

1. Get an `infra_internal` checkout
1. `cd data/gae`
1. `vim apps/luci-milo/channels.json`
1. Modify the "stable" version in
   [channels.json](https://chrome-internal.googlesource.com/infradata/gae/+/refs/heads/master/apps/luci-milo/channels.json)
   (e.g. by reusing the current staging version).
1. `./main.star`
1. Mail and land the CL.

TODO: Describe how to collect Milo release notes.
