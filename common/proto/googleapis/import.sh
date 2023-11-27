#!/bin/bash
# Copyright 2023 The LUCI Authors.

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at

#      http://www.apache.org/licenses/LICENSE-2.0

# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
set -e

# List of proto files within https://github.com/googleapis/googleapis to import.
PROTOS_TO_IMPORT=(
  google/api/annotations.proto
  google/api/client.proto
  google/api/field_behavior.proto
  google/api/field_info.proto
  google/api/http.proto
  google/api/launch_stage.proto
  google/api/resource.proto

  google/cloud/security/privateca/v1/resources.proto

  google/rpc/code.proto
  google/rpc/error_details.proto
  google/rpc/status.proto

  google/type/color.proto
  google/type/date.proto
  google/type/dayofweek.proto
  google/type/expr.proto
  google/type/latlng.proto
  google/type/money.proto
  google/type/postal_address.proto
  google/type/timeofday.proto
)

cd $(dirname "${BASH_SOURCE[0]}")

# Clear existing imported files.
rm -rf google

# Get pinned version of googleapis that the version of
# google.golang.org/genproto in go.mod pins.
genproto_version=$(grep "google.golang.org/genproto" ../../../go.sum | grep -v go.mod | awk '{print $2}' | sort -V | tail -n1)
genproto_mod_dir=$(go mod download -json google.golang.org/genproto | \
  python3 -c "import sys, json; print(json.load(sys.stdin)['Dir'])")
googleapis_version=$(cat "${genproto_mod_dir}/regen.txt")

echo "using googleapis hash: ${googleapis_version}"

# Grab the most recent checkout of "googleapis" repo.
CHECKOUT=".googleapis_checkout"
mkdir -p ${CHECKOUT}
pushd ${CHECKOUT}
git init
git remote rm origin || true
git remote add origin https://github.com/googleapis/googleapis
git config extensions.partialclone origin
git config remote.origin.fetch +refs/heads/master:refs/remotes/origin/master
git config remote.origin.partialclonefilter blob:none
git config pull.rebase true
git fetch origin
git pull origin --depth 1 ${googleapis_version}
git checkout origin/master
REVISION=$(git rev-parse HEAD)
popd

# Copy all requested files + LICENSE + revision information.
for PROTO in ${PROTOS_TO_IMPORT[*]}
do
  mkdir -p $(dirname ${PROTO})
  cp "${CHECKOUT}/${PROTO}" ${PROTO}
done
cp "${CHECKOUT}/LICENSE" google/
echo "${REVISION}" > google/REVISION
echo "${genproto_version}" > google/GENPROTO_REGEN

# Make sure all dependencies are copied too by trying to compile everything.
set +e
echo
echo "Compiling all imported files to make sure their dependencies exist..."
echo
protoc --proto_path=. --descriptor_set_out=/dev/null ${PROTOS_TO_IMPORT[*]}
if [ $? -ne 0 ]; then
  echo
  echo "protoc call failed. Examine its output and add missing *.proto files"
  echo "to PROTOS_TO_IMPORT. Keep doing that until it no longer complains."
  echo
  echo "DO NOT IGNORE THIS ERROR!"
  exit 1
fi
rm -rf ${CHECKOUT}
echo "Success!"
