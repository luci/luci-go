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

set -eu

cd "$(dirname $0)"

LUCI_ROOT=../../../../../../luci

echo "- Copy the proto files from luci-py"
cp ${LUCI_ROOT}/appengine/swarming/proto/api/*.proto api
cp ${LUCI_ROOT}/appengine/swarming/proto/config/*.proto config
cp ${LUCI_ROOT}/appengine/swarming/proto/jsonrpc/*.proto jsonrpc

echo "- Fix import paths"
# Fix the import path due to difference between the way Go and python process
# imports; Go doesn't allow relative import.
sed -i 's#"swarming\.proto"#"go.chromium.org/luci/swarming/proto/api/swarming.proto"#' api/*.proto
sed -i 's#"config\.proto"#"go.chromium.org/luci/swarming/proto/config/config.proto"#' config/*.proto
sed -i 's#"realms\.proto"#"go.chromium.org/luci/swarming/proto/config/realms.proto"#' config/*.proto

echo "- git commit to use to update README.md:"
cd ${LUCI_ROOT}
echo -n '  '
git rev-parse HEAD
