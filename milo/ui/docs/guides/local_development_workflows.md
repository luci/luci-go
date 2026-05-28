# Local Development Workflows

This guide describes how to run and develop the LUCI (Layered Universal
Continuous Integration) Milo UI locally.

## Prerequisites

To set up your environment, run the following commands.

### 1. Get the source

You may need
[depot_tools](https://chromium.googlesource.com/chromium/tools/depot_tools/+/HEAD/README.md)
([instructions here](https://commondatastorage.googleapis.com/chrome-infra-docs/flat/depot_tools/docs/html/depot_tools_tutorial.html#_setting_up)),
and from there follow
[this documentation](https://chromium.googlesource.com/infra/infra/+/HEAD/doc/source.md)
to get the `infra` repo (or a repo that contains the `infra` repo):

If you `cd` into the repo directory, you should see a structure like in the
following diagram, where only the relevant bits
for this guide are expanded.

```sh
<your_repo_dir>
├── build (…)
├── build_internal
├── chromium_infra (…)
├── data (…)
├── gcloud (…)
├── infra
│   └── go                      ← (we will call this $GO_DIR)
│       └── src
│           └── go.chromium.org
│               └──luci
│                  └── milo     ← (we will call this $MILO_DIR)
│                      └── ui   ← (we will call this $UI_DIR)
│   └── (…)
├── infra_internal (…)
├── puppet (…)
├── recipes-py (…)
├── release_scripts (…)
└── systems (…)
```

By convention we refer to `<your_repo_dir>` as `infra`.

If you cd into `<your_repo_dir>`, copy and paste this into your terminal so
that you can copy and paste the rest of the commands in this guide:

```sh
$ export GO_DIR="$(PWD)/infra/go"
$ export MILO_DIR="$(PWD)/infra/go/src/go.chromium.org/luci/milo"
$ export UI_DIR="$(PWD)/infra/go/src/go.chromium.org/luci/milo/ui"
```

### 1. Activate the Infra Environment

The `infra` repository uses a custom bootstrap environment. To activate it:

```sh
# Navigate to the go directory in your infra checkout:
$ cd "${GO_DIR}"

# Run the environment script:
$ eval `./env.py`
```

### 2. Install Node.js Dependencies
Go to the Milo UI directory and install the package dependencies:

```sh
# Clean install dependencies:
$ cd "${UI_DIR}"
$ npm ci
```

---

## Start a Local AppEngine Server

If you need to test a Local GAE (Google App Engine) server, or use
`/rpcexplorer` for testing RPCs (Remote Procedure Calls):

WARNING: For most UI development you don't need to run the app engine server
locally. You only need to run the app engine server if you are making changes
to the server, which is extremely rare

```sh
# Run the local AppEngine frontend server:
$ cd "${MILO_DIR}"
$ go run frontend/main.go \
  -cloud-project luci-milo-dev \
  -auth-service-host chrome-infra-auth-dev.appspot.com \
  -milo-host localhost:8080
```

---

## Start a Local UI Server

To start a [Vite](https://vitejs.dev) local dev server, run

```sh
$ cd "${UI_DIR}"
$ npm run dev
```

The local development server only serves the SPA (Single Page Application)
assets. It sends pRPC and HTTP REST requests to staging servers hosted on GCP
(Google Cloud Platform).

Check the [.env.development](../../.env.development) file for instructions to
configure the target servers and other local development settings.

If you need to tweak the values within `.env.development` do so within
`.env.development.local` (which you many need to create and place in the same
directory as `.env.development`); The `.local` one will override the values
from the non-`.local` one.

This will prevent from accidentally submitting pointers to unintended endpoints.

### Login on a local UI server

When developing with a local UI server, the login flow is different from a
deployed version. Click the **Login** button in your local browser interface
(top right corner), and follow the on-screen instructions.

---

## Serving remotely

If your code is on a different computer, you can see it on your local computer
by creating an ssh tunnel that forwards the port you are serving on.

From your local computer (i.e. laptop), run:

```sh
$ ssh [your-username]@[your-remote] -L 8080:localhost:8080
```

Leave this command running in the background for as long as you want. While
the ssh connection remains stablished, you'll be able to see the UI at:
http://localhost:8080/ui/ from your local browser.

---

## Deploy a demo to a GAE dev instance

### Option A: Build and Deploy Everything (Recommended for pRPC changes)
Use `make up-dev` if your changes include Go/pRPC backend updates:

```sh
# Run the up-dev command:
infra_dir/infra/go/src/go.chromium.org.luci/milo/ui$ make up-dev
```

### Option B: Deploy the UI Demo (Fastest)
If you have only modified frontend code, you can deploy just the UI:

```sh
$ cd "${UI_DIR}"
# If you haven't installed/updated the dependencies.
$ npm ci
# Deploy the UI-only demo:
$ make deploy-ui-demo
```

> [!NOTE]
> `make deploy-ui-demo` is faster than `make up-dev`, but:
> 1. It does not automatically run `npm ci`. Ensure your local dependencies are
>    installed and up-to-date.
> 2. The deployed UI demo will always send pRPC requests to
>    `staging.milo.api.luci.app`.

---

## Others

Check the following files for other utility and helper scripts:
* [Makefile](Makefile) (Milo UI tasks)
* [Parent Makefile](../Makefile) (Milo service-level tasks)
* [package.json](package.json) (available NPM scripts)
