# Copyright 2019 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.
#
# This Dockerfile specifies the commands for building testing images, consisting
# of local code.

FROM golang:1.11-alpine

RUN apk update \
  && apk add git

COPY . /go/src/app

# Fetch local code.
RUN go get -v app/frontend \
  && go install app/frontend

ENTRYPOINT ["/go/bin/frontend"]
