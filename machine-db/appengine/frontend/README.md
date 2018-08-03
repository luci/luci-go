# Machine Database

These are instructions for setting up development for and testing changes to the Machine Database.


## Setting up

* In [infra/go](https://chromium.googlesource.com/infra/infra/+/master/go/) run the following to activate the go environment:
```shell
cd ../../../../../../
eval `./env.py`
cd -
```

* To set up the UI (frontend and RPC explorer):
```shell
cd ../../../web
./web.py install
./web.py build rpcexplorer
cd -
```

## Testing

* Use [gae.py](https://chromium.googlesource.com/infra/luci/luci-py/+/master/appengine/components/tools/gae.py) to upload to dev:
```shell
gae.py upload --app-id machine-db-dev
```
