# Machine Database

These are instructions for setting up and testing Machine Database.


## Setting up

* In infra/go run the following to activate the go environment:
```
eval `./env.py`
```

* To set up the RPC explorer:
```
cd ../../../web
./web.py build rpcexplorer
```

* Install [Google App Engine SDK](https://cloud.google.com/appengine/downloads).

* Run `npm install -g bower`

* Run `bower install` in the frontend directory to make sure you have all the dependencies installed.

## Testing

* Use gae.py to upload to dev:
`gae.py upload --app-id machine-db-dev`
