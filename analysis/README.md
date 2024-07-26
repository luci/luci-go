# LUCI Analysis

LUCI Analysis is a system designed to understand and reduce the impact of test
failures.

## Prerequisites

Commands below assume you are running in the infra environment.
To enter the infra env (via the infra.git checkout), run:
```
eval infra/go/env.py
```

## Local Development flow

To run the server locally, first authorize as the correct GCP project (you should only need to do this once):
```
gcloud config set project luci-analysis-dev
gcloud auth application-default login
```

Authenticate in LUCI and in CIPD:

1. In LUCI Analysis's `frontend` directory run:
   ```
   luci-auth login -scopes "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email"
   ```
2. In the same directory run:
   ```
   cipd auth-login
   ```

Once the GCP project is authorized, in one terminal start esbuild to rebuild the UI code after any changes:

```
cd frontend/ui
npm run watch
```

To run the server, in another terminal use:
```
cd frontend
go run main.go \
 -cloud-project luci-analysis-dev \
 -spanner-database projects/luci-analysis-dev/instances/dev/databases/luci-analysis-dev \
 -auth-service-host chrome-infra-auth-dev.appspot.com \
 -luci-analysis-host 127.0.0.1:8800 \
 -default-request-timeout 10m0s \
 -buganizer-mode disable \
 -config-local-dir ../configs \
 -encrypted-cookies-expose-state-endpoint
```

`-default-request-timeout` is needed if exercising cron jobs through the admin
portal as cron jobs run through the /admin/ endpoint attract the default
timeout of 1 minute, instead of the 10 minute timeout of the /internal/ endpoint
(hit by GAE cron jobs when they are actually executing).

`-buganizer-mode` is needed when running cron jobs that create/update buganizer
issues (e.g. update-analysis-and-bugs). Set the mode to `disable` to prevent the
cron job from filing buganizer bugs.

Note that `-config-local-dir` is required only if you plan on modifying config
and loading it into Cloud Datastore via the read-config cron job accessible via
http://127.0.0.1:8900/admin/portal/cron for testing. Omitting this, the server
will fetch the current config from Cloud Datastore (as periodically refreshed
from LUCI Config Service).

You may also be able to use an arbitrary cloud project (e.g. 'dev') if you
setup Cloud Datastore emulator and setup a config for that project under
configs.

## Running UI tests

You can run the UI tests by:
```
cd frontend/ui
npm run test
```

## Run Spanner integration tests using Cloud Spanner Emulator

### Install Cloud Spanner Emulator

#### Linux

The Cloud Spanner Emulator is part of the bundled gcloud, to make sure it's installed:

```
cd infra
gclient runhooks
eval `./go/env.py`
which gcloud # should show bundled gcloud
gcloud components list # should see cloud-spanner-emulator is installed
```

### Run tests

From command line, first set environment variables:

```
export INTEGRATION_TESTS=1
```

Then run go test as usual. For example:

```
go test go.chromium.org/luci/analysis/...
```

## Run UI linter

To run the UI code linter (note: this will also try to auto-fix issues), run:

```
cd frontend/ui
npm run lint
```

To additionally auto-fix issues, run:

```
cd frontend/ui
npm run lint-fix
```

## Regenerating UI proto bindings

You can regenerate typescript bindings for proto files by running:
```
cd frontend/ui
npm run gen_proto
```

## Deployment

### Developer Testing {#test-deployment}

LUCI Analysis uses `gae.py` for deployment of the GAE instances for developer
testing (e.g. of local changes).

First, enter the infra env (via the infra.git checkout):
```
eval infra/go/env.py
```

Then use the following commands to deploy:
```
cd frontend/ui
npm run build
gae.py upload -A luci-analysis-dev default api
```

### Dev and Prod Instances

The dev and prod instances are managed via
[LUCI GAE Automatic Deployment (Googlers-only)](http://go/luci/how_to_deploy.md).

#### Make a production release

1. Make sure that you have an `infra_internal` checkout.
2. Navigate to `data/gae` directory under your base checkout directory, should be `~/infra`.
3. Pull the latest with `git rebase-update` or `git checkout main && git pull`.
4. Create a new branch `git new-branch <NAME>` or `git -b <NAME>`.
5. run `./scripts/promote.py luci-analysis --canary --stable --commit`.
6. Upload the CL `git cl upload`.
7. Request approval from space: LUCI Test War Room.

