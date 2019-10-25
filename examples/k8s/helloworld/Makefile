# Copyright 2019 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

# All commands here are exclusively for local development. Images produced here
# shouldn't really be pushed anywhere.

build-binary:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./docker .

build-image: build-binary
	docker build -t helloworld ./docker

run-image: build-image
	docker run --rm \
		-p 8800:8800 \
		-p 8900:8900 \
		-v ~/.config/chrome_infra/auth:/tokens \
		helloworld:latest \
			-http-addr :8800 \
			-admin-addr :8900 \
			-token-cache-dir /tokens
