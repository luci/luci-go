#!/bin/bash
# Copyright 2019 The LUCI Authors.
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

set -e

# Get tip of HEAD.
rm -rf google.golang.org
git clone https://go.googlesource.com/protobuf google.golang.org/protobuf
cd google.golang.org/protobuf

# Record its revision for posterity.
git rev-parse HEAD > ../../REVISION

# Apply local patches.
git apply --check ../../patches/*.patch
git am ../../patches/*.patch

# Trim unnecessary fat, leaving essentially only non-test *.go and legal stuff.
rm -rf benchmarks
rm -rf cmd
rm -rf compiler
rm -rf encoding/testprotos
rm -rf internal/cmd
rm -rf internal/testprotos
rm -rf prototest
rm -rf testing
rm .gitignore .travis.yml go.mod go.sum *.md

find . -type f -name '*_test.go' -delete
find . -type f -name '*.bash' -delete

rm -rf .git

# Presubmit scripts in luci-go are too dumb to recognize vendor/* or to skip
# linting it if it hasn't been touched.
gofmt -w -s .
