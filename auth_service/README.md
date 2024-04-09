Auth Service
------------

Auth Service will manages and distribute data and configuration used for authorization decisions performed by services in a LUCI cluster.

This is a replacement the [GAE v1 version of Auth Service](https://chromium.googlesource.com/infra/luci/luci-py/+/HEAD/appengine/auth_service).


Running locally
---------------

Targeting the real datastore:

```
cd auth_service/services/frontend
go run main.go -cloud-project chrome-infra-auth-dev

```

The server will be available at http://localhost:8800.


Uploading to GAE for adhoc testing
----------------------------------

Prefer to test everything locally. If you must deploy to GAE, use:

```
cd auth_service
gae.py upload --target ${USER} -A chrome-infra-auth-dev --app-dir services defaultv2 backendv2

```

This will upload versions for both `defaultv2` and `backendv2` services,
with target version name `${USER}`.

Note that it doesn't switch the default serving version. Use Cloud Console
or `gcloud app services set-traffic` to switch the serving version of
`defaultv2` and `backendv2` services. Be careful not to touch `default` and
`backend`. They are deployed from Python code.


Production deployment
---------------------

Deployment to staging and production are performed by [gae-deploy] builder.
Deploying directly to production using `gae.py` is strongly ill-advised.

[gae-deploy]: https://ci.chromium.org/p/infradata-gae/builders/ci/gae-deploy
