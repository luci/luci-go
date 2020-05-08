# Copyright 2014 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
projdir := $(patsubst %/,%,$(dir $(mkfile_path)))

dev:
	gae.py devserver --app-dir $(projdir)/frontend -A luci-milo-dev -- --host 0.0.0.0 --port 8082 --admin_port 7999 --log_level debug

up-dev:
	gcloud app deploy ./frontend/appengine/app.yaml ./frontend/appengine/service-pubsub.yaml --project luci-milo-dev --no-promote -v $$USER

up-dev-default:
	gcloud app deploy ./frontend/appengine/app.yaml --project luci-milo-dev --no-promote -v $$USER

up-dev-resultui: build-resultui
	gcloud app deploy ./frontend/resultui/service-resultui.yaml --project luci-milo-dev --no-promote -v $$USER

test:
	go test ./...

train: gen
	go test ./... -test.generate

gen:
	go generate ./...

build-resultui:
	cd frontend/resultui && make build
