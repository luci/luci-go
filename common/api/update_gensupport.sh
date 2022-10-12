#!/bin/bash
# Copyright 2020 The LUCI Authors.
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

cd $(dirname $0)

# It is important to use the exact same code as used by
# `google-api-go-generator` binary which is pulled in tools.go from
#`google.golang.org/api` module. Otherwise we may be generating incompatible
# code.
api_mod_dir=$(go mod download -json google.golang.org/api | \
  python3 -c "import sys, json; print(json.load(sys.stdin)['Dir'])")

# Copy internal/gensupport package from there into our tree.
rm -f internal/gensupport/*.go
cp ${api_mod_dir}/internal/gensupport/* internal/gensupport
chmod -R u+w internal/gensupport

# We don't really care about tests nor can really run them with a partial copy.
rm internal/gensupport/*_test.go
