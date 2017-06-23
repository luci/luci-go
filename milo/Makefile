# Copyright 2014 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
projdir := $(patsubst %/,%,$(dir $(mkfile_path)))

dev:
	gae.py devserver --app-dir $(projdir)/frontend/appengine -- --host 0.0.0.0 --port 8082 --admin_port 7999 --log_level debug

# This is intentionally not dependent on the others below to avoid
# asking for user confirmation multiple times.
up: up-all up-dispatch

up-all:
	gae.py upload -A luci-milo --app-dir $(projdir)/frontend/appengine default pubsub api

up-default:
	gae.py upload -A luci-milo --app-dir $(projdir)/frontend/appengine default

up-pubsub:
	gae.py upload -A luci-milo --app-dir $(projdir)/frontend/appengine pubsub

up-api:
	gae.py upload -A luci-milo --app-dir $(projdir)/frontend/appengine api

up-dispatch:
	appcfg.py -A luci-milo update_dispatch $(projdir)/frontend/appengine

up-dev: up-dev-logs
	gae.py upload -A luci-milo-dev --app-dir $(projdir)/frontend/appengine default pubsub api
	appcfg.py -A luci-milo-dev update_dispatch $(projdir)/frontend/appengine

up-dev-default:
	gae.py upload -A luci-milo-dev --app-dir $(projdir)/frontend/appengine default

up-dev-logs:
	gcloud app deploy --project luci-milo-dev --version dev $(projdir)/logs/module-logs.yaml

test:
	go test ./...

train: gen
	go test ./... -test.generate

gen:
	go generate ./...
