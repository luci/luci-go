# Contribution
## Run a Local Instance
Before you start, you need to create two configuration files.

${PROJECT_ROOT}/dev-configs/configs.json
```javascript
// Configurations of the App.
{
    "RESULT_DB": {
        "HOST": "<ResultDB Host Name>" // e.g. "staging.results.api.cr.dev"
    },
    "OAUTH2": {
        "CLIENT_ID": "<OAuth2 Client ID>"
    }
}
```
Replace `<OAuth2 Client ID>` with the OAuth 2.0 client ID you get from the [luci-milo-dev GCP console](https://pantheon.corp.google.com/apis/credentials?project=luci-milo-dev).

${PROJECT_ROOT}/dev-configs/local-dev-configs.json
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

Then you can run the following command to start a local instance.
```
npm install
npm run dev
```
This will start a webpack-dev-server instance running at localhost:8080.
