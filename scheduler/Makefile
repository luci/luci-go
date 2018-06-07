# Copyright 2018 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
projdir := $(patsubst %/,%,$(dir $(mkfile_path)))

# This is intentionally not dependent on the others below to avoid
# asking for user confirmation multiple times.
up:
	gae.py upload -A luci-scheduler --app-dir $(projdir)/appengine/frontend

up-dev:
	gae.py upload -A luci-scheduler-dev --app-dir $(projdir)/appengine/frontend

switch:
	gcloud --project luci-scheduler app versions migrate $$(gae.py version -A luci-scheduler --app-dir $(projdir)/appengine/frontend)

switch-dev:
	gcloud --project luci-scheduler-dev app versions migrate $$(gae.py version -A luci-scheduler-dev --app-dir $(projdir)/appengine/frontend)

test:
	go test ./...

gen:
	go generate ./...

versions-cleanup-dev:
	gae.py cleanup -A luci-scheduler-dev --app-dir $(projdir)/appengine/frontend

versions-cleanup:
	gae.py cleanup -A luci-scheduler --app-dir $(projdir)/appengine/frontend
