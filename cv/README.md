# LUCI Change Verifier

LUCI Change Verifier (CV) is the service that will replace
CQ. CQ is the legacy service that verifies patches before they are submitted.

TODO(crbug.com/920494): Update this after migrating to the new service.

## What's here?

As of Aug 2020:

 - [api/]: Protobuf files specifying the config schema, BigQuery schema, etc.
 - [appengine/]: A GAE app, under construction.
 - [appengine/config/]: Config validation logic.

## Links

 - The legacy CQ code is in [infra_internal](https://chrome-internal.googlesource.com/infra/infra_internal/+/master/infra_internal/services/cq/README.md).

 - More internal docs are found at [go/luci/cq](https://goto.google.com/luci/cq).
