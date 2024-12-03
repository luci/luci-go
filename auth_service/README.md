# Auth Service

Auth Service manages and distributes data and configuration used for
authorization decisions performed by services in a LUCI cluster.

This is the replacement of the
[GAE v1 version of Auth Service](https://chromium.googlesource.com/infra/luci/luci-py/+/f5769d3/appengine/auth_service).

#### Table of contents
- [Overview](#overview)
- [Configuration distributed by Auth Service aka AuthDB](#configuration-distributed-by-auth-service-aka-authdb)
    - [Groups graph](#groups-graph)
    - [IP allowlists](#ip-allowlists)
    - [OAuth client ID allowlist](#oauth-client-id-allowlist)
    - [Security configuration for internal LUCI RPCs](#security-configuration-for-internal-luci-rpcs)
- [API surfaces](#api-surfaces)
    - [Groups API](#groups-api)
    - [AuthDB replication](#authdb-replication)
    - [Hooking up a LUCI service to receive AuthDB updates](#hooking-up-a-luci-service-to-receive-authdb-updates)
- [External dependencies](#external-dependencies)
- [Developer guide](#developer-guide)
    - [Running locally](#running-locally)
    - [Uploading to GAE for adhoc testing](#uploading-to-gae-for-adhoc-testing)
    - [Production deployment](#production-deployment)

## Overview

It is part of the control plane, and as such it is **not directly involved** in
authorizing every request to LUCI. Auth Service merely tells other services how
to do it, and they do it themselves.

There's one Auth Service per LUCI cluster. When a new LUCI service joins the
cluster it is configured with the location of Auth Service, and Auth Service is
configured with the identity of the new service. This is one time setup. During
this initial bootstrap the new service receives the full snapshot of all
authorization configuration, which it can cache locally and start using right
away. Then the new service can either register a web hook to be called by Auth
Service when configuration changes, or it can start polling Auth Service for
updates itself.

Whenever authorization configuration changes (e.g. a group is updated), Auth
Service prepares a new snapshot of all configuration, makes it available to
all polling clients, and pushes it to all registered web hooks until they
acknowledge it.

If Auth Service goes down, everything remains operational, except the
authorization configuration becomes essentially "frozen" until Auth Service
comes back online and resumes distributing it.

## Configuration distributed by Auth Service (aka AuthDB)

This section describes what exactly is "data and configuration used for
authorization decisions".

See AuthDB message in
[replication.proto](../server/auth/service/protocol/components/auth/proto/replication.proto) for
all details.

### Groups graph

Groups are how the lowest layer of ACLs is expressed in LUCI, e.g. a service
may authorize some action to members of some group. More complex access control
rules use groups as building blocks.

Each group has a name (global to the LUCI cluster), a list of identity strings
it includes directly (e.g. `user:alice@example.com`), a list of nested groups,
and a list of glob-like patterns to match against identity strings
(e.g. `user:*@example.com`).

An identity string encodes a principal that performs an action. It's the result
of authentication stage. It looks like `<type>:<id>` and can represent:
  * `user:<email>` - Google Accounts (end users and service accounts).
  * `anonymous:anonymous` - callers that didn't provide any credentials.
  * `project:<project>` - a LUCI service acting in a context of some LUCI
    project when calling some other LUCI service.
  * `bot:<hostname>` - used only by Swarming, individual bots pulling tasks.
  * **[Deprecated]** `bot:whitelisted-ip` - callers authenticated exclusively
    through an IP allowlist.
  * **[Deprecated]** `service:<app-id>` - GAE application authenticated via
    `X-Appengine-Inbound-Appid` header.

Note that various UIs and configs may omit the `user:` prefix; it is implied if
no other prefix is provided. For example, `*@example.com` in a UI actually means
`user:*@example.com`.

### IP allowlists

An IP allowlist is a named set of IPv4 and IPv6 addresses. They are primarily
used in Swarming when authorizing RPCs from bots. IP allowlists are defined in
the `ip_allowlist.cfg` service configuration file.

### OAuth client ID allowlist

This is a list of Google [OAuth client IDs] recognized by the LUCI cluster. It
lists various OAuth clients (standalone binaries, web apps, AppScripts, etc)
that are allowed to send end-user OAuth access tokens to LUCI. The OAuth client
ID allowlist is defined in the `oauth.cfg` service configuration file.

[OAuth client IDs]: https://www.oauth.com/oauth2-servers/client-registration/client-id-secret/

### Security configuration for internal LUCI RPCs

These are various bits of configuration (defined partially in the `oauth.cfg`
and in the `security.cfg` service config files) that are centrally distributed
to services in a LUCI cluster. Used to establish mutual trust between them.

## API surfaces

### Groups API

This is a pRPC service to examine and modify the groups graph. It is used
primarily by Auth Service's own web frontend (i.e. it is mostly used by humans).
See the [RPC Explorer]. Alternatively, read the
[Groups service proto definitions] and the corresponding
[Groups service implementation].

Services that care about availability **must not** use this API for
authorization checks. It has no performance or availability guarantees. If you
use this API, and your service goes down because Auth Service is down, it is
**your fault**.

Instead, services should use AuthDB replication (normally through a LUCI client
library such as [go.chromium.org/luci/server]) to obtain and keep up-to-date the
snapshot of all groups, and use it locally without hitting Auth Service on every
request. See the next couple of sections for more information.

[RPC Explorer]: https://defaultv2-dot-chrome-infra-auth.appspot.com/rpcexplorer
[Groups service proto definitions]: ./api/rpcpb/groups.proto
[Groups service implementation]: ./impl/servers/groups/server.go
[go.chromium.org/luci/server]: https://godoc.org/go.chromium.org/luci/server

### AuthDB replication

All authorization configuration internally is stored in a single Cloud Datastore
entity group. This entity group has a revision number associated with it, which
is incremented with every transactional change (e.g. when groups are updated).

For every revision, Auth Service:
1. creates a consistent snapshot of AuthDB at this particular
revision
1. updates its internal latest AuthDB snapshot so it can be served over
`/auth_service/api/v1/authdb/revisions/<revId>` and the RPC method
`auth.service.AuthDB/GetSnapshot`.
1. signs the AuthDB and uploads it to a preconfigured Google Storage bucket
1. sends a PubSub notification to a preconfigured PubSub topic; and finally
1. uploads the snapshot (via POST HTTP request) to all registered web hooks
until all of them acknowledge it.

It is distributed in a such diverse way for backward compatibility with variety
of AuthDB client implementations (in historical order):
  * Web hooks are how Python GAE services consume AuthDB via [components.auth]
    client library.
  * PubSub and `/auth_service/api/v1/authdb/revisions/...` endpoint is how
    Go GAE services consume AuthDB via [go.chromium.org/luci/server/auth]
    client.
  * `/auth_service/api/v1/authdb/revisions/...` is also polled by Gerrit `cria/`
    groups plugin.
  * Google Storage dump is consumed by Go GKE services (that don't have
    non-ephemeral storage to cache AuthDB in). This is also done via
    [go.chromium.org/luci/server/auth] client configured by
    [go.chromium.org/luci/server].


### Hooking up a LUCI service to receive AuthDB updates

In all cases the goal is to let Auth Service know the identity of a new service
and let the new service know the location of Auth Service. How this looks
depends on where the new service is running and what client library it uses.

Python GAE services should use [components.auth] library. Once the service is
deployed, perform this one-time setup:
  * If you are member of `administrators` group, open
    `https://<auth-service>.appspot.com/auth/services` in a browser, put app ID
    of the new service in `Add service` section and click `Generate linking
    URL`.
  * If you are not a member of `administrators` group, ask an administrator to
    do it on your behalf and share the resulting link with you.
  * If you are GAE admin of the new service, follow the link, it will ask for
    confirmation. Confirm. If it succeeds, you are done.
  * If you are not a GAE admin, ask the admin to visit the link.
  * During this process the new service receives an initial snapshot of AuthDB
    and registers a web hook to be called when AuthDB changes.

Go GAE services should use [go.chromium.org/luci/server/auth] and hooking them
to an Auth Service looks different:
  * Make sure Google Cloud Pub/Sub API is enabled in the cloud project that
    contains the new service.
  * Add GAE service account (`<appid>@appspot.gserviceaccount.com`) of the
    new service to `auth-trusted-services` group (or ask an administrator to do
    it for you).
  * As a GAE admin, open `https://<appid>.appspot.com/admin/portal/auth_service`
    and put `https://<auth-service>.appspot.com` into `Auth Service URL` field.
  * Click `Save Settings`. If it succeeds, you are done.
  * During this process the new service receives an initial snapshot of AuthDB,
    creates a new PubSub subscription, asks Auth Service to grant it access to
    the AuthDB notification PubSub topic and subscribes to this topic, so it
    knows when to refetch AuthDB from `/.../authdb/revisions/...` endpoint.

Go GKE/GCE services should use [go.chromium.org/luci/server]. There's no notion
of "GAE admin" for them, and no dynamic settings. Instead the configuration is
done through command line flags:
  * Add the service account of the new service to `auth-trusted-services` group
    (or ask an administrator to do it for you).
  * Pass `-auth-service-host <auth-service>.appspot.com` flag when launching the
    server binary.
  * If it starts and responds to `/healthz` checks, you are done.
  * Beneath the surface `-auth-service-host` is used to derive a Google Storage
    path to look for AuthDB snapshots (it can also be provided directly via
    `-auth-db-dump`). When server starts it tries to fetch AuthDB from there.
    On permission errors it asks Auth Service to grant it access to the AuthDB
    dump and tries again. This normally happens only on the first run. On
    subsequent runs, the new service doesn't send any RPCs to Auth Service
    **at all** and just periodically polls Google Storage bucket. Note that
    AuthDB is cached only in memory, but this is fine since we assume it is
    always possible to fetch it from Google Storage (i.e. Google Storage becomes
    a hard dependency, which has very high availability).

## External dependencies

Auth Service depends on following services:
  * App Engine standard: the serving environment.
  * Cloud Datastore: storing the state (including groups graph).
  * Cloud PubSub: sending AuthDB update notifications to authorized clients.
  * Cloud Storage: saving AuthDB dumps to be consumed by authorized clients.
  * Cloud IAM: managing PubSub ACLs to allow authorized clients to subscribe.
  * LUCI Config: receiving own configuration files.

---

## Developer guide

### Running locally

Targeting the real datastore:

```
cd auth_service/services/frontend
go run main.go -cloud-project chrome-infra-auth-dev

```

The server will be available at http://localhost:8800.

### Uploading to GAE for adhoc testing

Prefer to test everything locally. If you must deploy to GAE, use:

```
cd auth_service
gae.py upload --target ${USER} -A chrome-infra-auth-dev --app-dir services defaultv2 backendv2

```

This will upload versions for both `defaultv2` and `backendv2` services,
with target version name `${USER}`.

Note that it doesn't switch the default serving version. Use Cloud Console
or `gcloud app services set-traffic` to switch the serving version of
`defaultv2` and `backendv2` services. Be careful not to touch `default` and
`backend`. They are deployed from Python code.


### Production deployment

Deployment to staging and production are performed by [gae-deploy] builder.
Deploying directly to production using `gae.py` is strongly ill-advised.

[gae-deploy]: https://ci.chromium.org/p/infradata-gae/builders/ci/gae-deploy
