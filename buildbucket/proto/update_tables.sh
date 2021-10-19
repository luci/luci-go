#!/bin/sh
# Copyright 2021 The LUCI Authors.
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

# This little script is just to remember the incantation to update the bigquery
# schema. If you don't know what this is, you don't need to run it (and likely
# don't have permission to anyhow).

# Fist, make sure you run up-to-date bqschemaupdater,
# since it's not installed by default in Infra's Go Env.
go install go.chromium.org/luci/tools/cmd/bqschemaupdater

# Actually update BQ schemas, -dev before prod.
bqschemaupdater -message buildbucket.v2.Build -table cr-buildbucket-dev.raw.completed_builds
bqschemaupdater -message buildbucket.v2.PRPCRequestLog -table cr-buildbucket-dev.sandbox.prpc_request_log -partitioning-field creation_time -partitioning-expiration "720h" # 30d
bqschemaupdater -message buildbucket.v2.Build -table cr-buildbucket.raw.completed_builds
bqschemaupdater -message buildbucket.v2.PRPCRequestLog -table cr-buildbucket.sandbox.prpc_request_log -partitioning-field creation_time -partitioning-expiration "720h" # 30d
