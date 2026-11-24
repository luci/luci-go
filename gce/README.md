# GCE Provider Service

This service is responsible for managing the lifecycle of Google Compute Engine (GCE) Virtual Machines, primarily for use as Swarming bots.

## Configuration

The service uses configuration files to define VM shapes, disk images, network settings, and swarming pool assignments.

-   **Proto Definitions:** The core configuration schema is defined in `api/config/v1/config.proto`. This includes messages for `VM`, `Disk`, `NetworkInterface`, etc.
-   **User Configs:** Current user configurations are located in the `infra_superproject` repository at [`data/config/configs/gce-provider/`](http://cs/h/chromium/infra/infra_superproject/+/main:data/config/configs/gce-provider/). Older configs can also be seen at go/gce-provider-config.

### Regeneration After Proto Changes

When you modify `api/config/v1/config.proto`, you MUST regenerate the Go bindings by running:

```bash
go generate ./...
```

from within the `go.chromium.org/luci/gce/api/config/v1` directory. This will update the `*.pb.go` and `*.pb.discovery.go` files.

### Virtual Display (`enable_display`)

Setting `enable_display: true` inside a `VM` attributes block attaches a [GCE virtual display device](https://cloud.google.com/compute/docs/instances/enable-instance-virtual-display) (`DisplayDevice.EnableDisplay: true`) when creating the instance. This is used for headless VMs without a physical GPU that require an emulated display adapter (such as Windows UEFI VMs running interactive GUI or browser tests with the `google-compute-engine-driver-gga` driver).

Restrictions and operational notes:
- Supported on x86_64 non-bare-metal, non-GPU GCE machine series (on both `GCP_CHANNEL_STABLE` and `GCP_CHANNEL_ALPHA`).
- Rejected by config validation on Arm machine series (e.g., `t2a-*`, `c3a-*`, `c4a-*`, `e4a-*`, `c5a-*`, `n4a-*`, `n5a-*`, `a4x-*`), bare-metal machine types (`*-metal`), GPU machine series (`a2-*`, `a3-*`, `a4-*`, `g2-*`, `g4-*`), instances using the Intel Sandy Bridge minimum CPU platform (`"Intel Sandy Bridge"`, `"sandybridge"`, or `"intel-sandybridge"`), and Confidential VMs (`enable_confidential_compute: true`).
- `enable_display: true` requires `compute.instances.updateDisplayDevice` IAM permission on the target GCP project (custom roles missing it fail with HTTP 403 on `Instances.Insert`).
- Setting or changing `enable_display` applies only when creating or recreating VMs (`lifetime` expiry); GCE does not update running VMs in place.

## Service Architecture

The service consists of several components:

-   **`appengine/frontend`**: Handles incoming API requests.
-   **`appengine/backend`**: Contains cron jobs and task queue handlers for:
    -   Creating new VM instances.
    -   Deleting terminated or orphaned instances.
    -   Auditing existing instances against the configuration.
    -   Managing instance deadlines and drainage.
-   **Task Queues:** The service utilizes Cloud Tasks to manage asynchronous operations like instance creation and deletion.

## Deployment

The service is deployed on Google App Engine in the following GCP projects:

-   **Production:** `gce-provider`
-   **Development:** `gce-provider-dev`
