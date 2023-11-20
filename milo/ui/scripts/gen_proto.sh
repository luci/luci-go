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

cd "$(dirname "$0")"

rm -rf ../src/proto
mkdir ../src/proto
cd ../../../../../

# Use ts-proto instead of the official grpc-web because it is RPC framework
# agnostic. This is curcial because we use a non-standard protocol (pRPC) for
# client-server communication.
protoc \
  --plugin=./go.chromium.org/luci/milo/ui/node_modules/.bin/protoc-gen-ts_proto \
  -I=./go.chromium.org/luci/common/proto/googleapis \
  -I=./ \
  --ts_proto_out=./go.chromium.org/luci/milo/ui/src/proto \
  \
  `# Add '.pb' so it can be ignored by presubmit upload checks.` \
  --ts_proto_opt=fileSuffix=.pb \
  \
  --ts_proto_opt=forceLong=string,esModuleInterop=true \
  --ts_proto_opt=removeEnumPrefix=true,unrecognizedEnum=false \
  --ts_proto_opt=useDate=string,useReadonlyTypes=true \
  \
  ./go.chromium.org/luci/analysis/proto/v1/clusters.proto \
  ./go.chromium.org/luci/analysis/proto/v1/test_history.proto \
  ./go.chromium.org/luci/bisection/proto/v1/analyses.proto \
  ./go.chromium.org/luci/buildbucket/proto/builder_service.proto \
  ./go.chromium.org/luci/buildbucket/proto/builds_service.proto \
  ./go.chromium.org/luci/milo/proto/v1/rpc.proto \
  ./go.chromium.org/luci/resultdb/proto/v1/resultdb.proto \
  ./go.chromium.org/luci/swarming/proto/api_v2/swarming.proto \
