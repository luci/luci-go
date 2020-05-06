# ResultUI
Frontend to ResultDB.

# Running a Local Instance
Before you start, you need to create a configuration file, local-dev-config.json, at the root of the project.
It should have the following content.
```json
{
    "client_id": "<OAuth2 Client ID>",
    "result_db": {
        "host": "<ResultDB Host Name>"
    }
}
```
Replace `<OAuth2 Client ID>` with the client ID you get from the luci-milo-dev GCP console.


Then you can run the following command to start a local instance.
```
npm install
npm run dev
```
This will start a webpack-dev-server instance running at localhost:8080.
