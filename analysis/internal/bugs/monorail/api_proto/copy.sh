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

# Delete existing generated files.
rm *.proto
rm *.pb.go

# Copy proto files from monorail directory.
cp ../../../../../../../../../appengine/monorail/api/v3/api_proto/*.proto .

# Replace go_package.
sed -i 's/infra\/monorailv2\/api\/v3/go.chromium.org\/luci\/analysis\/internal\/bugs\/monorail/g' *.proto

# Fixup imports to other files.
sed -i 's/api\/v3/go.chromium.org\/luci\/analysis\/internal\/bugs\/monorail/g' *.proto
