Swarming server Go code
-----------------------

To deploy from a local checkout:

```
gae.py upload -A chromium-swarm-dev default-go
```

To run locally:

```
cd server/cmd
go run main.go -shared-hmac-secret devsecret://aaaa
```
