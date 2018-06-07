# Copyright 2018 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
projdir := $(patsubst %/,%,$(dir $(mkfile_path)))
app_dir := $(projdir)/appengine/frontend

# Production AppEngine instance.
prod := 'luci-scheduler'
# Dev AppEngine instance.
dev := 'luci-scheduler-dev'

# This is intentionally not dependent on the others below to avoid
# asking for user confirmation multiple times.
up:
	gae.py upload -A $(prod) --app-dir $(app_dir)

up-dev:
	gae.py upload -A $(dev) --app-dir $(app_dir)

switch:
	gae.py switch -A $(prod) --app-dir $(app_dir)

switch-dev:
	gae.py switch -A $(dev) --app-dir $(app_dir)

test:
	go test ./...

gen:
	go generate ./...

versions-cleanup:
	gae.py cleanup -A $(prod) --app-dir $(app_dir)

versions-cleanup-dev:
	gae.py cleanup -A $(dev) --app-dir $(app_dir)
