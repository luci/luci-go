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

function import_dir() {
  local src_dir=${1}
  local dst_dir=${2}

  echo "- ${dst_dir}: cleaning existing files"
  rm -f ${dst_dir}/*.proto
  rm -f ${dst_dir}/*.pb.go
  rm -f ${dst_dir}/*_dec.go
  rm -f ${dst_dir}/pb.discovery.go

  echo "- ${dst_dir}: copy the proto files from luci-py"
  cp ${src_dir}/*.proto ${dst_dir}

  echo "- ${dst_dir}: fix import paths"
  sed -i 's#import "proto/#import "go.chromium.org/luci/swarming/proto/#' ${dst_dir}/*.proto
}

function add_luci_file_metadata() {
  local proto=${1}
  local cfg_file=${2}

  local fragment=(
    "\n"
    "import \"go.chromium.org/luci/common/proto/options.proto\";\n"
    "\n"
    "option (luci.file_metadata) = {\n"
    "  doc_url: \"https://config.luci.app/schemas/services/swarming:${cfg_file}\";\n"
    "};"
  )
  local joined=`printf '%s' "${fragment[@]}"`

  sed -i "s#option go_package = \"\\(.*\\)\";#option go_package = \"\\1\";\\n${joined}#" ${proto}
}

import_dir ${LUCI_ROOT}/appengine/swarming/proto/api_v2 api_v2
import_dir ${LUCI_ROOT}/appengine/swarming/proto/config config
import_dir ${LUCI_ROOT}/appengine/swarming/proto/internals internals
import_dir ${LUCI_ROOT}/appengine/swarming/proto/plugin plugin

echo "- adding luci.file_metadata option"
add_luci_file_metadata config/bots.proto bots.cfg
add_luci_file_metadata config/config.proto settings.cfg
add_luci_file_metadata config/pools.proto pools.cfg

echo "- regenerating Go code"
go generate ./...

echo "- updating README.md"
pushd ${LUCI_ROOT} > /dev/null
LUCI_REV=`git rev-parse HEAD`
popd > /dev/null
sed -i "s#Revision: .*#Revision: ${LUCI_REV}#" README.md
