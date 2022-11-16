# Copyright 2014 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
projdir := $(patsubst %/,%,$(dir $(mkfile_path)))

up-dev: build-ui
	gae.py upload -p ./ -A luci-milo-dev --target-version $$USER

up-dev-default: build-ui
	gae.py upload -p ./ -A luci-milo-dev default --target-version $$USER

test:
	go test ./...

train: gen
	go test ./... -test.generate

gen:
	go generate ./...

build-ui:
	cd ui && make build
