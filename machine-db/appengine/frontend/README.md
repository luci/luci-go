# Crimson (machine-db) UI

This is a UI for the Crimson (machine-db) service.


## Setting up

* In infra/go run the following to activate the go environment:
```
eval `./env.py`
```

* Install [Google App Engine SDK](https://cloud.google.com/appengine/downloads).

* Run `npm install -g bower`

* Run `bower install` in the frontend directory to make sure you have all the dependencies installed.

## Testing

* Use gae.py to upload to dev:
`gae.py upload --app-id machine-db-dev`
