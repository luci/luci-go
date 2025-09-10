#!/usr/bin/env bash
# Copyright 2025 The LUCI Authors.
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

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
cd ${SCRIPT_DIR}

function update {
  cd $1

  mv go.mod.template go.mod
  mv go.sum.template go.sum

  go get -tool $2
  go mod tidy

  mv go.mod go.mod.template
  mv go.sum go.sum.template

  cd ..
}

update modern google.golang.org/protobuf/cmd/protoc-gen-go@latest
update ancient github.com/golang/protobuf/protoc-gen-go@latest
