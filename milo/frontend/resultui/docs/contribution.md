# Contribution
## Run a Local Instance
Follow the steps below to run a local instance.

### 1. Create ${PROJECT_ROOT}/dev-configs/configs.json
```javascript
// Configurations of the App.
{
    "RESULT_DB": {
        "HOST": "<ResultDB Host Name>" // e.g. "staging.results.api.cr.dev"
    },
    "BUILDBUCKET": {
        "HOST": "<Buildbucket Host Name>" // e.g. "cr-buildbucket-dev.appspot.com"
    }
}
```

### 2. Create ${PROJECT_ROOT}/dev-configs/local-dev-configs.json
```javascript
// Configurations of the webpack-dev-server.
{
    "milo": {
        // webpack-dev-server will proxy requests that should be handled
        // by the Milo server to this URL.
        "url": "<Milo URL>"  // e.g. "https://luci-milo-dev.appspot.com"
    }
}
```

### 3. Create & trust a Self-signed certificate.
1. Run `make gen_cert`.
2. Trust `dev-configs/cert.pem`.

### 4. Start the local instance.
Run the following command to start a local instance.
```sh
npm install  # install node modules
npm run dev  # start the dev server
```
This will start a webpack-dev-server instance running at localhost:8080.
