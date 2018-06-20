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
	cd $(app_dir) && gae.py upload -A $(prod)

up-dev:
	cd $(app_dir) && gae.py upload -A $(dev)

switch:
	cd $(app_dir) && gae.py switch -A $(prod)

switch-dev:
	cd $(app_dir) && gae.py switch -A $(dev)

test:
	go test ./...

gen:
	go generate ./...

versions-cleanup:
	cd $(app_dir) && gae.py cleanup -A $(prod)

versions-cleanup-dev:
	cd $(app_dir) && gae.py cleanup -A $(dev)
