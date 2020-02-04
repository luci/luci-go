# Copyright 2020 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

# All commands here are exclusively for local development and this file can
# safely be deleted if it's not useful. It has mostly informational purpose.
#
# Images produced here shouldn't really be pushed anywhere. The real images are
# produced by Chrome Ops CI pipeline:
# https://chromium.googlesource.com/infra/infra/+/master/build/images/README.md.

build-binary:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./docker .

build-image: build-binary
	docker build -t logdog_collector ./docker
