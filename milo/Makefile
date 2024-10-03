# Copyright 2014 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
projdir := $(patsubst %/,%,$(dir $(mkfile_path)))

# Both the `api`, the `default`, and `ui` service should be uploaded at least
# once. Otherwise, the browser code will try to call a dev API that doesn't
# exist. This, unfortunately, is unintuitive and triples the deployment time.
#
# TODO: we can
# * redirect calls to unmatched demo -dev version API service to the default
#   -dev version API service, or
# * separate the API service to a standalone GAE service so their deployments
#   are decoupled.
up-dev: build-ui
	gae.py upload -p ./ -A luci-milo-dev -f

test:
	go test ./...

train: gen
	go test ./... -test.generate

gen:
	go generate ./...

build-ui:
	cd ui && make build
