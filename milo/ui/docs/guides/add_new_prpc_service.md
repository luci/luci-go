# Adding a new PRPC service to LUCI UI

## Add a new host

If LUCI UI is not currently using any services on the host of the new service, you need to [add a new host](add_new_host.md) to LUCI UI before adding the new service.

## Generate proto bindings

LUCI UI uses generated proto bindings for calling PRPC services.

To tell LUCI UI to about the new service, add the proto file with your service definition to the end of `luci/milo/ui/scripts/gen_proto.sh`.

Once this is complete, you can regenerate the LUCI UI proto bindings with `npm run gen-proto` in the `ui` directory.

You will need to regenerate the proto bindings any time your UI code needs to use an updated version of the service proto.

You can find the generated proto files in `ui/src/proto`.

## Trouble shooting
### Unable to find a dependency proto file (File not found).
This happens when the proto files you added to the list imports files from a
location not known to the code generator.

For instance, the proto files in
[chromiumos/config](https://source.chromium.org/chromium/infra/infra_superproject/+/main:infra/go/src/go.chromium.org/chromiumos/config/proto/;drc=0caf1814ce046e09e14889aa86e16f1505e0962c) imports files with [chromiumos/config](https://source.chromium.org/chromium/infra/infra_superproject/+/main:infra/go/src/go.chromium.org/chromiumos/config/proto/;drc=0caf1814ce046e09e14889aa86e16f1505e0962c) as the base directory.
`-I=./go.chromium.org/chromiumos/config/proto` needs to be passed to `protoc`.
Otherwise, `protoc` will not be able to locate the files in [chromiumos/config](https://source.chromium.org/chromium/infra/infra_superproject/+/main:infra/go/src/go.chromium.org/chromiumos/config/proto/;drc=0caf1814ce046e09e14889aa86e16f1505e0962c).
