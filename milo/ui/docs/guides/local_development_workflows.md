
# Local Development Workflows

## Prerequisites

You need to run the following commands to setup the environment.

```sh
# Activate the infra env (via the infra.git checkout):
cd /path/to/infra/checkout
eval infra/go/env.py

# Install the dependencies.
cd /path/to/this/directory
npm ci
```

## Start a local AppEngine server

TODO: add instructions

For the simple case of testing an rpc using `/rpcexplorer`:

```sh
go run main.go -cloud-project luci-milo-dev -auth-service-host chrome-infra-auth-dev.appspot.com -milo-host localhost:8080
```

## Start a local UI server

To start a [Vite](https://vitejs.dev) local dev server, run

```sh
npm run dev
```

The local dev server only serves the SPA assets. It sends pRPC and HTTP REST
requests to staging servers (typically hosted on GCP). Check
[.env.development](../../.env.development) for instructions to configure the target
servers and other local development settings.

### Login on a local UI server

When developing with a local UI server, the login flow is different from a
deployed version. Click the login button and you should see the instruction.

## Deploy a demo to GAE dev instance

This deploys your local code to GAE as a dev instance that uses real auth
and can be accessed by other people.

```sh
cd luci/milo/ui # or `cd luci/milo`.
make up-dev
```

Alternatively, you can deploy the demo using the following commands.

```sh
cd luci/milo/ui
npm ci # if you haven't installed/updated the dependencies.
make deploy-ui-demo
```

`make deploy-ui-demo` is faster than `make up-dev`. However,

1. it does not install npm dependencies. You need to ensure they are installed
   and up-to-date yourselves. And,
2. the deployed UI demo will always send pRPC requests to
   `staging.milo.api.luci.app`. If your demo includes pRPC changes, use
   `make up-dev` instead.

If you use `gae.py` to deploy, you need to deploy all services at least once.
Otherwise, the browser code will try to call a dev API that doesn't exist.

```sh
cd luci/milo/ui
npm run build
# Upload all services. In particular, the `api` service should be uploaded at
# least once.
gae.py upload -p ../ -A luci-milo-dev
# After that, you can deploy `ui-new` only for shorter deployment time, if
# there's no code changes to other services.
gae.py upload -p ../ -A luci-milo-dev ui-new
```

## Others

Check the [Makefile](Makefile), the [parent Makefile](../Makefile), and the
`"scripts"` section in [package.json](package.json) for more available commands.
