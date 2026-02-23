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
