Swarming server Go code
-----------------------

To deploy from a local checkout:

```
gae.py upload -A chromium-swarm-dev
```

To run locally:

```
cd server/cmd/default
go run main.go \
  -cloud-project chromium-swarm-dev \
  -shared-hmac-secret devsecret://aaaa \
  -primary-tink-aead-key devsecret-gen://tink/aead
```
