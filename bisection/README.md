# LUCI Bisection

LUCI Bisection (formerly GoFindit) is the culprit finding service for compile and test failures for Chrome Browser.

This is the rewrite in Golang of the Python2 version of Findit (findit-for-me.appspot.com).

## Local Development

To run the server locally, firstly you need to authenticate
```
gcloud config set project chops-gofindit-dev
gcloud auth application-default login
```
and
```
luci-auth login -scopes "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email"
```

### Building the Frontend

In another terminal window, build the project with watch for development:
```
cd frontend/ui
npm run watch
```
This will build the React app. If left running, local changes to the React app
will trigger re-building automatically.

To run the frontend unit tests,
```
cd frontend/ui
npm test
```

### Running LUCI Bisection

In the root bisection directory, run
```
go run main.go -cloud-project luci-bisection-dev -primary-tink-aead-key sm://tink-aead-primary
```

This will start a web server running at http://localhost:8800.
Navigate to this URL using your preferred browser.
Once you "log in", the LUCI Bisection frontend should load.

### Developer Test Deployment

LUCI Bisection uses `gae.py` for manual deployment of the GAE instances for developer
testing (e.g. of local changes).

In the root bisection directory, run
```
eval `../../../../env.py`
make deploy
```

## Releasing

The dev and prod instances are managed via
[LUCI GAE Automatic Deployment (Googlers-only)](http://go/luci/how_to_deploy.md).

### Dev instance

Releases are automatically pushed to luci-bisection-dev on commit by the
[gae-deploy](https://ci.chromium.org/p/infradata-gae/builders/ci/gae-deploy)
builder.

### Prod instance

To push to prod:
1. Get an `infra_internal` checkout
1. Change to the `data/gae` directory:
    * `cd data/gae`
1. Get the latest changes from the `main` branch:
    * `git checkout main && git pull`
1. Switch to a local branch:
    * `git checkout -b <local-branch-name>`
1. Create the CL to update the "canary" and "stable" version:
    1. `./scripts/promote.py luci-bisection --canary --stable --commit`
    1. `git cl upload`
    * this will create a CL like [this one](https://chrome-internal-review.googlesource.com/c/infradata/gae/+/5008853)
1. Mail and land the CL.
    * If you want to rollback prod to the previous version, rollback this CL.
