# Fleet Console UI Client

This is the folder for the client side code for the Fleet Console.

For overall docs on the Fleet Console, see: [go/fleet-console](http://go/fleet-console)

## To run

This codebase is hosted as part of Milo UI. To run the frontend, see the overall [Milo dev docs](https://source.chromium.org/chromium/infra/infra_superproject/+/main:infra/go/src/go.chromium.org/luci/milo/ui/docs/guides/local_development_workflows.md).

After running the Milo server locally, the Fleet Console UI will be available at: <http://localhost:8080/ui/fleet/labs/devices>

### Set up SSH tunnel to Cloudtop

You can easily set up a tunnel from your local machine to to your instance via this command:

```sh
ssh -L 8080:localhost:8080 ${cloudtop-name}.c.googlers.com
```

This enables you to access the UI locally at <http://localhost:8080/ui/fleet/labs/devices>.

## Running tests

To run TS tests for just the Fleet Console (from the `ui/` root dir):

```sh
npm test -- ./src/fleet/
```

To make it more convenient to code, you can re-run tests automatically based
on file changes:

```sh
npm test -- ./src/fleet/ --watch
```

### Generate coverage

To check the test coverage of the codebase:

```sh
npm test -- ./src/fleet --collectCoverageFrom=src/fleet/**/* --coverage
```

The generated coverage report will also be available as a website in the file
location `ui/coverage/lcov-report/index.html`.

You can find this file in your file system and open it in your browser.

The URL will be something like:

`file:///{YOUR_INFRA_CHECKOUT_PARENT_DIR}/infra/infra/go/src/go.chromium.org/luci/milo/ui/coverage/lcov-report/index.html`

If working with VSCode on a Cloudtop, using the Live Preview option will work to view the html file.
