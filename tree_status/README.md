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
  -auth-service-host chrome-infra-auth-dev.appspot.com \
  -spanner-database projects/luci-tree-status-dev/instances/dev/databases/luci-tree-status-dev \
  -frontend-client-id 713057630411-eqah8ap1ptgnf4nnepk10sutqg0msiv1.apps.googleusercontent.com
```

You can test the RPCs using the [rpcexplorer](http://127.0.0.1:8800/rpcexplorer).

## Deploy demo instance with local changes to AppEngine

```
gae.py upload --target-version ${USER} -A luci-tree-status-dev
```

## Deploy to staging

Tree status is automatically deployed to staging by LUCI CD every time a CL is submitted.

## Deploy to prod

1. Make sure that you have an `infra_internal` checkout.
2. Navigate to `data/gae` directory under your base checkout directory.
3. Pull the latest with `git rebase-update` or `git checkout main && git pull`.
4. Create a new branch `git new-branch <NAME>` or `git -b <NAME>`.
5. run `./scripts/promote.py luci-tree-status --canary --stable --commit`.
6. Upload the CL `git cl upload`.
7. Request approval from space: LUCI Test War Room.

## Add a new tree

To add a new tree, you need to add an entry into LUCI Tree Status config.

Example CL: https://chrome-internal-review.googlesource.com/c/infradata/config/+/7631947

If you set `use_default_acls: true`, the tree will use the default ACL policy:

-   Googlers have full read access to the tree, including the PII (emails of the
    creators of the statuses).

-   Googlers have write access to the tree. This means they can create a new
    status.

-   Other authenticated users, but not Googlers, have read access to the tree,
    but they don't see PII.

There is a
[work-in-progress](https://buganizer.corp.google.com/issues/349178432) to allow
setting up customized ACL policy for your tree. This section will be updated
with detailed instructions after the work is completed.
