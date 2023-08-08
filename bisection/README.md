# LUCI Bisection

LUCI Bisection (formerly GoFindit) is the culprit finding service for compile and test failures for Chrome Browser.

This is the rewrite in Golang of the Python2 version of Findit (findit-for-me.appspot.com).

## Local Development

To run the server locally, firstly you need to authenticate
```
gcloud config set project luci-bisection-dev
gcloud auth application-default login
```
and
```
luci-auth login -scopes "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email"
```

### Building the Frontend

The frontend code under the root bisection directory has been deprecated.
See `milo/ui/src/bisection` for bisection frontend code.

### Running LUCI Bisection

In the root bisection directory, run
```
go run main.go -cloud-project luci-bisection-dev -primary-tink-aead-key sm://tink-aead-primary -config-service-host luci-config.appspot.com -luci-analysis-project luci-analysis-dev
```

This will start a web server running at http://localhost:8800.

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
