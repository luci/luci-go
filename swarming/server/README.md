Swarming server Go code
-----------------------

To deploy from a local checkout:

```
gae.py upload -A chromium-swarm-dev default-go
```

To run locally using fake bots and tasks (but with real Datastore and real RBE):

```
# Terminal 1: launch the server locally and keep it running.
cd server/cmd/default
go run main.go \
  -cloud-project chromium-swarm-dev \
  -shared-hmac-secret devsecret://aaaa \
  -primary-tink-aead-key devsecret-gen://tink/aead \
  -expose-integration-mocks

# Terminal 2: launch a fake bot and keep it running.
cd server/testing/fakebot
go run main.go -bot-id fake-bot-1 -pool "${USER}-local-test"

# Terminal 3: launch a task and see it handled by the fake bot.
cd server/testing/fakesubmit
go run main.go -pool "${USER}-local-test"
```

Note that these tests hit real RBE instance (by default `default_instance` in
`chromium-swarm-dev` project), so they may theoretically interfere with other
similar tests running on other machines or even with real `chromium-swarm-dev`
RBE traffic. `pool` dimension can be used to namespace them.
