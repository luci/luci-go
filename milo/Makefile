# Copyright 2014 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
projdir := $(patsubst %/,%,$(dir $(mkfile_path)))

dev:
	gae.py devserver --app-dir $(projdir)/appengine -- --host 0.0.0.0 --port 8082 --admin_port 7999 --log_level debug

# This is intentionally not dependent on the others below to avoid
# asking for user confirmation multiple times.
up: up-all up-dispatch

up-all:
	gae.py upload -A luci-milo --app-dir $(projdir)/appengine default pubsub api

up-default:
	gae.py upload -A luci-milo --app-dir $(projdir)/appengine default

up-pubsub:
	gae.py upload -A luci-milo --app-dir $(projdir)/appengine pubsub

up-api:
	gae.py upload -A luci-milo --app-dir $(projdir)/appengine api

up-dispatch:
	appcfg.py -A luci-milo update_dispatch $(projdir)/appengine/frontend

up-dev:
	gae.py upload -A luci-milo-dev --app-dir $(projdir)/appengine default pubsub api
	appcfg.py -A luci-milo-dev update_dispatch $(projdir)/appengine/frontend

test:
	go test ./...

train: gen
	go test ./... -test.generate

gen:
	go generate ./...
