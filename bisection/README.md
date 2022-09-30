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
To push to prod, the steps are:

1. Get an `infra_internal` checkout
2. `cd data/gae`
3. To get the latest changes: `git checkout main && git pull`
4. `vim apps/luci-bisection/channels.json`
5. Modify the "stable" version in
   [channels.json](https://chrome-internal.googlesource.com/infradata/gae/+/HEAD/apps/luci-bisection/channels.json)
   (e.g. by reusing the current staging version).
6. Run `./main.star` to regenerate the Makefile.
7. Create a CL and add release notes to the description, as follows:
    1. Get the commit hashes from the old and new versions. E.g. in channels.json,
       if you changed the stable version from "`12097-7a7a05d`" to "`12104-214c2d1`",
       the old commit hash is `7a7a05d` and the new commit hash is `214c2d1`. These
       correspond to commits in the
       [infra/luci/luci-go](https://chromium.googlesource.com/infra/luci/luci-go/) repo.
    2. Run git log between these two commits:
        ```
        git log 7a7a05d..214c2d1 --date=short --first-parent --format='%ad %ae %s'
        ```
    3. Add the resulting command line and output to the CL description. Example:
       https://crrev.com/i/2962041
8. Mail and land the CL.
