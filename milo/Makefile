# Copyright 2014 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
projdir := $(patsubst %/,%,$(dir $(mkfile_path)))

dev:
	gae.py devserver --app-dir $(projdir)/frontend -A luci-milo-dev -- --host 0.0.0.0 --port 8082 --admin_port 7999 --log_level debug

# This is intentionally not dependent on the others below to avoid
# asking for user confirmation multiple times.
up: up-all

up-all: build-resultui
	gae.py upload -A luci-milo --app-dir $(projdir)/frontend default pubsub resultui

up-default:
	gae.py upload -A luci-milo --app-dir $(projdir)/frontend default

up-pubsub:
	gae.py upload -A luci-milo --app-dir $(projdir)/frontend pubsub

up-resultui: build-resultui
	gae.py upload -A luci-milo --app-dir $(projdir)/frontend resultui

up-dev:
	gae.py upload -A luci-milo-dev --app-dir $(projdir)/frontend default pubsub

up-dev-default:
	gae.py upload -A luci-milo-dev --app-dir $(projdir)/frontend default

up-dev-resultui: build-resultui
	gcloud app deploy ./frontend/resultui/service-resultui.yaml --project luci-milo-dev --no-promote -v $$USER

switch:
	gae.py switch -A luci-milo --app-dir $(projdir)/frontend

switch-dev:
	gae.py switch -A luci-milo-dev --app-dir $(projdir)/frontend

test:
	go test ./...

train: gen
	go test ./... -test.generate

gen:
	go generate ./...

versions-cleanup-dev:
	gae.py cleanup -A luci-milo-dev --app-dir $(projdir)/frontend

versions-cleanup:
	gae.py cleanup -A luci-milo --app-dir $(projdir)/frontend

build-resultui:
	cd frontend/resultui && make build
