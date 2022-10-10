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

It is a AppEngine Flex instance using "custom" runtime, since Flex is the only
supported type of AppEngine instance that can stream responses to the clients,
and we do that when clients read unfinalized logs. It is a custom runtime,
because Flex doesn't support Go runtimes newer than go1.15. To use a newer
version we need to build a Docker image ourselves (which requires specifying
"custom" runtime).

### Services

The [services](services/) service exposes management endpoints to the instance's
microservices, notably the [Collector](../../../server/cmd/logdog_collector) and
[Archivist](../../../server/cmd/logdog_archivist) microservices. These endpoints
are used to coordinate the microservice-managed aspects of the log stream
lifecycle.

### Static

The [static](static/) service hosts static content, including:
* The LogDog Web Application
* The LogDog Lightweight Stream Viewer
* `rpcexplorer`

## Deployment

Prefer to use [the deployment automation]. In particular, there's no other easy
way to deploy `logs` module, since it uses a custom Docker image (because the
latest GAE Flex Go runtime is stuck in the past on go1.15 version and we have to
bring our own image to use a newer version).

To deploy other modules from the local checkout, you can use `gae.py` tool.
E.g. to update all modules other than `logs`:

```shell
cd coordinator
gae.py upload -A luci-logdog-dev default services static
gae.py switch -A luci-logdog-dev
```

[the deployment automation]: https://chrome-internal.googlesource.com/infradata/gae
