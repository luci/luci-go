#!/bin/bash
# Copyright 2023 The LUCI Authors.
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

die() {
  printf '%s\n' "$@" 1>&2
  exit 1
}

cd -- "$(dirname "$0")" || die 'cannot chdir'
[ -f gen_proto.sh ] || die 'failed to find own directory'

# Only delete generated bindings. Leave other files (e.g. OWNERS) in place.
find ../src/proto -name "*.pb.ts" -type f -delete
find ../src/proto -mindepth 1 -type d -empty -delete

cd ../../../../..
[ -d ./go.chromium.org ] || die 'no chromium.org directory'
[ -d ./infra ] || die 'no infra directory'
[ -d ./go.chromium.org/luci/common/proto/googleapis ] || die 'no googleapis protos'

# Use ts-proto instead of the official grpc-web because it is RPC framework
# agnostic. This is crucial because we use a non-standard protocol (pRPC) for
# client-server communication.
protoc \
  --plugin=./go.chromium.org/luci/milo/ui/node_modules/.bin/protoc-gen-ts_proto \
  \
  `# All the base directory to resolve the proto file dependencies from must be
   # listed here. Otherwise, protoc may not able to resolve the dependencies
   # listed in a proto file.` \
  -I=./go.chromium.org/luci/common/proto/googleapis \
  -I=./go.chromium.org/chromiumos/config/proto \
  -I=./ \
  \
  --ts_proto_out=./go.chromium.org/luci/milo/ui/src/proto \
  \
  `# Add '.pb' so it can be ignored by presubmit upload checks.` \
  --ts_proto_opt=fileSuffix=.pb \
  \
  `# Do not use ExactTypes because
   #  * It hinders productivity.
   #    * When used with useReadonlyTypes, it prevents the use of array literal
   #      unless some form of type casting is usesd. Type casting is verbose and
   #      breaks the type guarantee that ExactType is trying to enforce.
   #    * It uses complex recursive types that slow down the compiler.
   #  * It delivers very little benefit.
   #    * TypeScript already prevents declaring unknown properties in an object
   #      literal.
   #  * Fundamentally, we don't want this behavior.
   #    * When .fromPartial or .create is fed with a non-literal object, we want
   #      the extra properties to be silently discarded. Otherwise, adding a new
   #      field to a proto message will become a breaking change if the proto
   #      message is used to construct another proto message that is a supertype
   #      of this message.` \
  --ts_proto_opt=useExactTypes=false \
  \
  `# Do not use remove enum prefix because it will generate invalid enum variant
   # identifiers when the identifier begins with a digit after the prefix is
   # removed.` \
  --ts_proto_opt=removeEnumPrefix=false \
  \
  --ts_proto_opt=forceLong=string,esModuleInterop=true \
  --ts_proto_opt=unrecognizedEnum=false \
  --ts_proto_opt=useDate=string,useReadonlyTypes=true \
  \
  ./go.chromium.org/chromiumos/config/proto/chromiumos/test/api/device_manager_service.proto \
  ./go.chromium.org/luci/analysis/proto/v1/changepoints.proto \
  ./go.chromium.org/luci/analysis/proto/v1/clusters.proto \
  ./go.chromium.org/luci/analysis/proto/v1/metrics.proto \
  ./go.chromium.org/luci/analysis/proto/v1/projects.proto \
  ./go.chromium.org/luci/analysis/proto/v1/rules.proto \
  ./go.chromium.org/luci/analysis/proto/v1/test_history.proto \
  ./go.chromium.org/luci/analysis/proto/v1/test_variants.proto \
  ./go.chromium.org/luci/analysis/proto/v1/test_variant_branches.proto \
  ./go.chromium.org/luci/auth_service/api/rpcpb/authdb.proto \
  ./go.chromium.org/luci/auth_service/api/rpcpb/groups.proto \
  ./go.chromium.org/luci/bisection/proto/v1/analyses.proto \
  ./go.chromium.org/luci/buildbucket/proto/builder_service.proto \
  ./go.chromium.org/luci/buildbucket/proto/builds_service.proto \
  ./go.chromium.org/luci/luci_notify/api/service/v1/alerts.proto \
  ./go.chromium.org/luci/milo/proto/v1/rpc.proto \
  ./go.chromium.org/luci/resultdb/proto/v1/resultdb.proto \
  ./go.chromium.org/luci/resultdb/sink/proto/v1/sink.proto \
  ./go.chromium.org/luci/source_index/proto/v1/source_index.proto \
  ./go.chromium.org/luci/swarming/proto/api_v2/swarming.proto \
  ./go.chromium.org/luci/tree_status/proto/v1/tree_status.proto \
  ./go.chromium.org/luci/tree_status/proto/v1/trees.proto \
  ./infra/appengine/cr-rev/frontend/api/v1/service.proto \
  ./infra/appengine/sheriff-o-matic/proto/v1/alerts.proto \
  ./infra/fleetconsole/api/fleetconsolerpc/service.proto \
  ./infra/unifiedfleet/api/v1/rpc/fleet.proto \
