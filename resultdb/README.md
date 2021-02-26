## Run Spanner integration tests using Cloud Spanner Emulator

### Install Cloud Spanner Emulator

From command line, run: `make install-spanner-emulator`

### Run tests
From command line, first to set environment variables by running: 

```
export INTEGRATION_TESTS=1
export SPANNER_EMULATOR=1
```

Then run go test as usual.