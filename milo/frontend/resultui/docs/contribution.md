# Contribution
## Run a Local Instance
Before you start, you need to create a configuration file, local-dev-config.json, at the root of the ResultUI project.
It should have the following content.
```json
{
    "client_id": "<OAuth2 Client ID>",
    "result_db": {
        "host": "<ResultDB Host Name>"  // e.g. "staging.results.api.cr.dev"
    }
}
```
Replace `<OAuth2 Client ID>` with the OAuth 2.0 client ID you get from the [luci-milo-dev GCP console](https://pantheon.corp.google.com/apis/credentials?project=luci-milo-dev).

Then you can run the following command to start a local instance.
```
npm install
npm run dev
```
This will start a webpack-dev-server instance running at localhost:8080.
