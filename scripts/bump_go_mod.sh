#!/bin/bash
# Copyright 2022 The LUCI Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This script updates selected dependencies in go.mod to latest versions.
#
# Dependencies that can be added to this list:
#  * Base dependencies shared by majority of packages in luci-go.
#  * Dependencies critical for security.
#  * Dependencies with a track record of not causing troubles during updates.
#
# Ideally running this script should always be a stress-free experience.

set -e

# TODO(vadimsh): Add Google Cloud modules. They are interconnected in a way that
# breaks Go modules: e.g. updating only one of them breaking compilation of
# others (probably their go.mod isn't kept up-to-date). So they need to be
# updated all at once.

deps=(
  github.com/alicebob/miniredis/v2
  github.com/danjacques/gofslock
  github.com/dustin/go-humanize
  github.com/envoyproxy/protoc-gen-validate
  github.com/golang/protobuf
  github.com/gomodule/redigo
  github.com/google/go-cmp
  github.com/google/tink/go
  github.com/google/uuid
  github.com/gorhill/cronexpr
  github.com/jordan-wright/email
  github.com/julienschmidt/httprouter
  github.com/klauspost/compress
  github.com/luci/gtreap
  github.com/maruel/subcommands
  github.com/mattn/go-tty
  github.com/mgutz/ansi
  github.com/Microsoft/go-winio
  github.com/mitchellh/go-homedir
  github.com/op/go-logging
  github.com/pmezard/go-difflib
  github.com/protocolbuffers/txtpbfmt
  github.com/russross/blackfriday/v2
  github.com/sergi/go-diff
  github.com/smartystreets/assertions
  github.com/smartystreets/goconvey
  github.com/yosuke-furukawa/json5
  go.starlark.net
  google.golang.org/grpc
  google.golang.org/grpc/cmd/protoc-gen-go-grpc
  google.golang.org/protobuf
  gopkg.in/yaml.v2
)

for mod in ${deps[@]}; do
  echo go get -d ${mod}@latest
  go get -d ${mod}@latest
done

echo go mod tidy -compat=1.17
go mod tidy -compat=1.17
