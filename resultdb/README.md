## Run Spanner integration tests using Cloud Spanner Emulator

### Install Cloud Spanner Emulator

From command line, run: `sudo apt-get install google-cloud-sdk-spanner-emulator`

### Run tests

From command line, first set environment variables:

```
export INTEGRATION_TESTS=1
export SPANNER_EMULATOR=1
```

Then run go test as usual.
