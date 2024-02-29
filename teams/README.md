# LUCI Teams Service

This service maintains information about development teams and the tests they are
interested in. The data is used to tailor LUCI UI to users.

## Prerequisites

Commands below assume you are running in the infra environment.
To enter the infra env (via the infra.git checkout), run:
```
eval infra/go/env.py
```

## Running tests locally

```
INTEGRATION_TESTS=1 go test go.chromium.org/luci/teams/...
```

## Running locally

```
go run main.go \
  -cloud-project luci-teams-dev \
  -auth-service-host chrome-infra-auth-dev.appspot.com \
  -spanner-database projects/luci-teams-dev/instances/dev/databases/luci-teams-dev
```

You can test the RPCs using the [rpcexplorer](http://127.0.0.1:8800/rpcexplorer).

## Deploy demo instance with local changes to AppEngine

```
gae.py upload --target-version ${USER} -A luci-teams-dev
```

## Deploy to staging

LUCI Teams is automatically deployed to staging by LUCI CD every time a CL is submitted.

## Deploy to prod

LUCI Teams is manually deployed to prod by creating a CL in the data/gae repository.
