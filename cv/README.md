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


## Links

 - The legacy CQDaemon code is in
   [infra_internal](https://chrome-internal.googlesource.com/infra/infra_internal/+/main/infra_internal/services/cq/README.md).
 - Additional *Google-internal* docs are found at
   [go/luci/cq](https://goto.google.com/luci/cq).
