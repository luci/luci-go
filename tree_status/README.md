# LUCI Tree Status Service

This service maintains a open/closed status for each "tree" (e.g. chromium).  Trees are defined arbitrarily and do not have to correspond 1:1 with a git repo or any other concept.

Each status update optionally includes a text description of why the tree was open/closed.

The tree status is generally used to temporarily stop new CLs from being merged while gardeners fix a broken tree.

## Prerequisites

Commands below assume you are running in the infra environment.
To enter the infra env (via the infra.git checkout), run:
```
eval infra/go/env.py
```

## Running tests locally

```
INTEGRATION_TESTS=1 go test ./...
```

## Running locally

```
go run main.go \
  -cloud-project luci-tree-status-dev \
  -auth-service-host chrome-infra-auth-dev.appspot.com
```

You can test the RPCs using the [rpcexplorer](http://127.0.0.1:8800/rpcexplorer).

## Deploy demo instance with local changes to AppEngine

```
gae.py upload --target-version ${USER} -A luci-tree-status-dev
```

## Deploy to staging

Tree status is automatically deployed to staging by LUCI CD every time a CL is submitted.

## Deploy to prod

The prod instance of LUCI Tree Status has not yet been created.