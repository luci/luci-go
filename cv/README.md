# LUCI Change Verifier

LUCI Change Verifier (CV) is the LUCI microservice that is replacing LUCI
CQDaemon, while providing the same or better functionality.

CV is already responsible for starting and completing the Runs, which includes
CL Submission.

As of August 2021, CV work group is working on second milestone moving the
remaining functionalities off CQDaemon and onto CV (tracking bug
https://crbug.com/1225047).

TODO(crbug.com/1225047): update this doc.

## What's here?

 - [api](./api): Protobuf files specifying the public API of CV,
   such as per-project config spec, PubSub messages and RPCs definitions.
 - [appengine](./appengine): the entry point for a GAE app.
 - [internal](./internal): GAE-agnostic implementation details.


## How does this code end up in production?

tl;dr this the usual way for LUCI GAE apps. Roughly,

  1. CL lands in `luci-go` (this) repo.

  1. Autoroller rolls it into `infra/infra` repo
     ([example](https://crrev.com/9ffe1ba58c936d4edf5852da63c470325cf490e8)).

       * *pro-tip*: If autoroller is stuck, you can create your own DEPS roll
         with `roll-dep go/src/go.chromium.org/luci` in your
         `infra/infra` checkout.

  1. Tarball with only the necessary files is created by the Google-internal
     `infra-gae-tarballs-continuous`
     [builder](https://ci.chromium.org/p/infra-internal/builders/prod/infra-gae-tarballs-continuous).

  1. Newest tarball is automatically rolled into also Google-internal `infradata-gae`
     [repo](https://chrome-internal.googlesource.com/infradata/gae/)
     ([example](https://chrome-internal.googlesource.com/infradata/gae/+/e2dc66dbdff92a9dde31758f3f1945758bbaa39c)).

  1. Google-internal `gae-deploy`
     [builder](https://ci.chromium.org/p/infradata-gae/builders/ci/gae-deploy)
     auto-deploys the newest tarball to `luci-change-verifier-dev`
     [GAE project](https://luci-change-verifier-dev.appspot.com/).

  1. Someone manually bumps tarball version to deploy to production
     `luci-change-verifier` via a CL ([example](https://crrev.com/i/4083212)).

       * *Recommended* use `bump-dev-to-prod.py` tool to make such a CL.
         For example,
           ```
           cd gae/app/luci-change-verifier
           ./bump-dev-to-prod.py -- --bug $BUG -r <REVIEWER> -d -s
           ```

  1. The same Google-internal `gae-deploy`
     [builder](https://ci.chromium.org/p/infradata-gae/builders/ci/gae-deploy)
     deploys the desired version to production `luci-change-verifier`
     [GAE project](https://luci-change-verifier.appspot.com/).

## Links

 - The legacy CQDaemon code is in
   [infra_internal](https://chrome-internal.googlesource.com/infra/infra_internal/+/main/infra_internal/services/cq/README.md).
 - Additional *Google-internal* docs are found at
   [go/luci/cq](https://goto.google.com/luci/cq).
