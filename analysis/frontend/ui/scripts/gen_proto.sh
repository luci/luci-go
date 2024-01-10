# Copyright 2024 The LUCI Authors.
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
cd ../../../../../../ # cd to directory containing go.chromium.org/.

# Use ts-proto instead of the official grpc-web because it is RPC framework
# agnostic. This is curcial because we use a non-standard protocol (pRPC) for
# client-server communication.
protoc \
  --plugin=./go.chromium.org/luci/analysis/frontend/ui/node_modules/.bin/protoc-gen-ts_proto \
  -I=./go.chromium.org/luci/common/proto/googleapis \
  -I=./ \
  --ts_proto_out=./go.chromium.org/luci/analysis/frontend/ui/src/proto \
  \
  `# Add '.pb' so it can be ignored by presubmit upload checks.` \
  --ts_proto_opt=fileSuffix=.pb \
  \
  --ts_proto_opt=forceLong=string,esModuleInterop=true \
  --ts_proto_opt=removeEnumPrefix=true,unrecognizedEnum=false \
  --ts_proto_opt=useDate=string,useReadonlyTypes=true \
  --ts_proto_opt=lowerCaseServiceMethods=true \
  \
  ./go.chromium.org/luci/analysis/proto/v1/clusters.proto \
  ./go.chromium.org/luci/analysis/proto/v1/metrics.proto \
  ./go.chromium.org/luci/analysis/proto/v1/projects.proto \
  ./go.chromium.org/luci/analysis/proto/v1/rules.proto \
  ./go.chromium.org/luci/analysis/proto/v1/test_variants.proto