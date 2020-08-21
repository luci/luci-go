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
    },
    "OAUTH2": {
        "CLIENT_ID": "<OAuth2 Client ID>"
    }
}
```
Replace `<OAuth2 Client ID>` with the OAuth 2.0 client ID you get from the [luci-milo-dev GCP console](https://pantheon.corp.google.com/apis/credentials?project=luci-milo-dev).

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

### 3. Create ${PROJECT_ROOT}/dev-configs/dev-server.crt & dev-server.key
This certificate is used for a local HTTPS proxy.
Here's an example of creating a certificate using openssl.
1. Create a RSA-2048 key:   
`openssl genrsa -des3 -out dev-root-ca.key 2048`
2. Create a Root SSL certificate:   
`openssl req -x509 -new -nodes -key dev-root-ca.key -sha256 -days 1024 -out dev-root-ca.pem`
3. Trust the root SSL certificate.
4. Create a server certificate key:   
`openssl req -new -sha256 -nodes -out dev-server.csr -newkey rsa:2048 -keyout dev-server.key`
5. Create a v3.et file.   
```
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
```
6. Create a server certificate:   
`openssl x509 -req -in dev-server.csr -CA dev-root-ca.pem -CAkey dev-root-ca.key -CAcreateserial -out dev-server.crt -days 500 -sha256 -extfile v3.ext`
7. Move `dev-server.key` and `dev-server.crt` to `${PROJECT_ROOT}/dev-configs/`

### 4. Start the local instance.
Run the following command to start a local instance.
```sh
npm install  # install node modules
npm run dev  # start the dev server
```
This will start a webpack-dev-server instance running at localhost:8080.
