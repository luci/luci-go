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
      - [Go GAE/GKE/GCE services](#go-gaegkegce-services)
      - [GAE services using first-gen runtime](#deprecated-services-using-gae-first-gen-runtime)
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
See the [RPC Explorer (for chrome-infra-auth-dev)]. Alternatively, read the
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

[RPC Explorer (for chrome-infra-auth-dev)]: https://chrome-infra-auth-dev.appspot.com/rpcexplorer
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
  * PubSub and `/auth_service/api/v1/authdb/revisions/...` endpoint is how some
    Go GAE services consume AuthDB via [go.chromium.org/luci/server/auth]
    client.
  * `/auth_service/api/v1/authdb/revisions/...` is also polled by Gerrit `cria/`
    groups plugin.
  * Google Storage dump is consumed by both Go GAE service and GKE services (that don't have
    non-ephemeral storage to cache AuthDB in). This is also done via
    [go.chromium.org/luci/server/auth] client configured by
    [go.chromium.org/luci/server].


### Hooking up a LUCI service to receive AuthDB updates

In all cases the goal is to let Auth Service know the identity of a new service
and let the new service know the location of Auth Service. How this looks
depends on where the new service is running and what client library it uses.

#### Go GAE/GKE/GCE services

Go GAE/GKE/GCE services should use [go.chromium.org/luci/server]:
  1. Add the service account of the new service to `auth-trusted-services` group
    group by filing a [go/peepsec-bug](https://goto.google.com/peepsec-bug).
  1. Pass the `-auth-service-host <auth-service>.appspot.com` flag.
      * For GAE services, in its config file (`app.yaml` for the `default`
      service, `<service-name>.yaml` for additional services), pass the
      `-auth-service-host` flag in the app's entrypoint.
        * e.g. LUCI Analysis's [app.yaml](../analysis/frontend/app.yaml).
      * For GKE/GCE services, when launching the server binary.
        * If it starts and responds to `/healthz` checks, you are done.

How does it work?
  * The `-auth-service-host` flag is used to derive a Google Storage (GCS) path
  to look for an AuthDB snapshot.
  * The GCS storage path can also be provided directly by using the
  `-auth-db-dump` flag.
  * When the server starts, it will attempt to fetch the AuthDB from the
  configured GCS storage path.
    * If there are permission errors, the server will send a POST request to
    Auth Service's `/auth_service/api/v1/authdb/subscription/authorization`
    endpoint. This asks Auth Service to grant the service account authorization
    to:
      * subscribe to PubSub notifications for the latest AuthDB revision (note
      this authorization is not actually used by most GAE/GKE/GCE services); and
      * read the AuthDB dump in GCS.
    * After Auth Service grants the requested authorization, the server will:
      * try once more to fetch the AuthDB from the GCS dump.
    * This normally happens only on the first run.
  * On subsequent runs, the new service doesn't send any RPCs to Auth Service
  **at all** and just periodically polls Google Storage bucket.
  * Note that AuthDB is cached only in memory, but this is fine since we assume
  it is always possible to fetch it from Google Storage
    * i.e. Google Storage becomes a hard dependency, which has very high
    availability.

#### **[Deprecated]** Services using GAE first-gen runtime

New services should not use this method of receiving AuthDB updates. The method
is described below primarily for Auth Service developers' understanding.

How does it work?
* The service account must be in the `auth-trusted-services` group.
  * This allows the service to call the legacy REST API to get the entire
  AuthDB, `/auth_service/api/v1/revisions/<revId|latest>`.
  * Auth Service will deny access to callers not in either of the
  `auth-trusted-services` or `administrators` groups.
* The service has a configuration setting for which Auth Service URL to use,
e.g. `https://chrome-infra-auth.appspot.com`.
* When the server starts, it will:
  1. Fetch the revision of the latest AuthDB using the legacy REST API.
      * GET request to
      `<auth_service_url>/auth_service/api/v1/revisions/latest?skip_body=1`
  1. Fetch the entire AuthDB at that revision using the legacy REST API.
      * GET request to
      `<auth_service_url>/auth_service/api/v1/revisions/<revId>`
      * This revision of the AuthDB is then stored in the service's local
      datastore.
  1. Set up PubSub notifications.
      1. Check if the service has already subscribed to AuthDB changes.
          * Exit early if so.
      1. If not subscribed, the server will send a POST request to
      Auth Service's `/auth_service/api/v1/authdb/subscription/authorization`
      endpoint. This asks Auth Service to grant the service account
      authorization to:
          * subscribe to PubSub notifications for the latest AuthDB revision;
          and
          * read the AuthDB dump in GCS (note: this is not used).
      1. Once the authorization is granted, the service is now eligible to
      subscribe to the AuthDB revision PubSub topic and creates the
      subscription.
* By subscribing to the AuthDB revision PubSub topic in the initial run, the
service will know when to refetch the AuthDB from the legacy REST API, and will
update its local datastore with the new AuthDB.

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
