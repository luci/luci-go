# Weetbix

Weetbix is a system designed to understand and reduce the impact of test
failures.

This app follows the structure described in [Template for GAE Standard app].

## Local Development

To run the server locally, first authorize as the correct GCP project (you should only need to do this once):
```
gcloud config set project chops-weetbix-dev
gcloud auth application-default login
```

Authenticate in LUCI and in :

1. In Weetbix's `frontend` directory run:
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
 -cloud-project chops-weetbix-dev \
 -spanner-database projects/chops-weetbix-dev/instances/dev/databases/chops-weetbix-dev \
 -auth-service-host chrome-infra-auth-dev.appspot.com \
 -default-request-timeout 10m0s \
 -config-local-dir ../configs
```

`-default-request-timeout` is needed if exercising cron jobs through the admin
portal as cron jobs run through the /admin/ endpoint attract the default
timeout of 1 minute, instead of the 10 minute timeout of the /internal/ endpoint
(hit by GAE cron jobs when they are actually executing).

Note that `-config-local-dir` is required only if you plan on modifying config
and loading it into Cloud Datastore via the read-config cron job accessible via
http://127.0.0.1:8900/admin/portal/cron for testing. Omitting this, the server
will fetch the current config from Cloud Datastore (as periodically refreshed
from LUCI Config Service).

You may also be able to use an arbitrary cloud project (e.g. 'dev') if you
setup Cloud Datastore emulator and setup a config for that project under
configs.

You can run the UI tests by:
```
cd frontend/ui
npm run test
```

You can debug the UI unit tests by visiting the server started by the following command with your browser:
```
cd frontend/ui
npm run test-watch
```

## Deployment

### Developer Testing {#test-deployment}

Weetbix uses `gae.py` for deployment of the GAE instances for developer
testing (e.g. of local changes).

First, enter the infra env (via the infra.git checkout):
```
eval infra/go/env.py
```

Then use the following commands to deploy:
```
cd frontend/ui
npm run build
gae.py upload --target-version ${USER} -A chops-weetbix-dev
```

### Dev and Prod Instances

The dev and prod instances are managed via
[LUCI GAE Automatic Deployment (Googlers-only)](http://go/luci/how_to_deploy.md).

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

Then run go test as usual. For example

```
go test -v ./...
```

[Template for GAE Standard app]: https://chromium.googlesource.com/infra/luci/luci-go/+/HEAD/examples/appengine/helloworld_standard/README.md

