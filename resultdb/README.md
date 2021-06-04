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

#### Mac

* [Install Docker Desktop on Mac](https://docs.docker.com/docker-for-mac/install/)

> If you are a Google employee, follow [go/docker-for-mac](go/docker-for-mac) first.


### Run tests

From command line, first set environment variables:

```
export INTEGRATION_TESTS=1
export SPANNER_EMULATOR=1
```

Then run go test as usual.

> Note: If you run tests on Mac, please start Docker Desktop before running tests.
