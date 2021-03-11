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

> Note: As of Mar 3, 2021, the newest version of Docker Desktop (3.2.0) is having issues, please download 3.1.0 [here](https://docs.docker.com/docker-for-mac/release-notes/#docker-desktop-310).

* After Docker Desktop is installed, go to Preferences > Experimental Features and toggle off "Enable cloud experience"

### Run tests

From command line, first set environment variables:

```
export INTEGRATION_TESTS=1
export SPANNER_EMULATOR=1
```

Then run go test as usual.

> Note: If you run tests on mac, please start Docker Desktop before running tests.
