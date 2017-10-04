LogDog Coordinator
==================

The LogDog Coordinator is an AppEngine application that serves as a central
coordinating and interactive authority for a LogDog instance. The Coordinator
is responsible for:

* Supplying logs to end users through its logs API.
* Coordinating log stream state throughout its lifecycle.
* Handling Butler Prefix registration.
* Acting as a configuration authority for its deployment.
* Accepting stream registrations from **Collector** instances.
* Dispatching archival tasks to **Archivist** instances.

## Services

A **Coordinator** occupies the AppEngine space of a given cloud project, and
assumes ownership of that project's resources. It is composed of several
cooperative AppEngine services.

### Default

The [default](default/) service handles basic LUCI services. Most other requests are
redirected to other services by [dispatch.yaml](default/dispatch.yaml).

### Logs

The [logs](logs/) service exposes the
[Logs API](../../../api/endpoints/coordinator/logs/v1/) for log stream querying
and consumption.

It is a AppEngine Flex instance, since Flex is the only supported type of
AppEngine instance that can use gRPC, and gRPC is needed to read from BigTable.

### Services

The [services](services/) service exposes management endpoints to the instance's
microservices, notably the [Collector](../../../server/cmd/logdog_collector) and
[Archivist](../../../server/cmd/logdog_archivist) microservices. These endpoints
are used to coordinate the microservice-managed aspects of the log stream
lifecycle.

### Backend

The [backend](backend/) service hosts the [Tumble](/tumble) journaled processing
service, which handles log stream lifecycle transitions and deadlines.

### Static

The [static](static/) service hosts static content, including:
* The LogDog Web Application
* The LogDog Lightweight Stream Viewer
* `rpcexplorer`
