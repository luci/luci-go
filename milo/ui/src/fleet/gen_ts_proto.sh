#!/bin/bash
# Copyright 2026 The LUCI Authors.
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
[ -f gen_ts_proto.sh ] || die 'failed to find own directory'

# Go to the root of the src directory
cd ../../../../../..
[ -d ./go.chromium.org ] || die 'no chromium.org directory'
[ -d ./infra ] || die 'no infra directory'
[ -d ./go.chromium.org/luci/common/proto/googleapis ] || die 'no googleapis protos'

# Setup symlink hack for go.chromium.org/infra
cleanup() {
  unlink ./go.chromium.org/infra 1>/dev/null 2>/dev/null
}
unlink ./go.chromium.org/infra 1>/dev/null 2>/dev/null
ln -s ../infra ./go.chromium.org/infra || die 'failed to make infra symlink'
trap cleanup EXIT

# Generate TS bindings
protoc \
  --plugin=./go.chromium.org/luci/milo/ui/node_modules/.bin/protoc-gen-ts_proto \
  \
  -I=./go.chromium.org/luci/common/proto/googleapis \
  -I=./go.chromium.org/chromiumos/config/proto \
  -I=./ \
  \
  --ts_proto_out=./go.chromium.org/luci/milo/ui/src/proto \
  \
  --ts_proto_opt=fileSuffix=.pb \
  --ts_proto_opt=useExactTypes=false \
  --ts_proto_opt=removeEnumPrefix=false \
  --ts_proto_opt=forceLong=string,esModuleInterop=true \
  --ts_proto_opt=unrecognizedEnum=false \
  --ts_proto_opt=useDate=string,useReadonlyTypes=true \
  --ts_proto_opt=exportCommonSymbols=false \
  \
  ./go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.proto

echo "Successfully generated fleetconsole TS protos."
