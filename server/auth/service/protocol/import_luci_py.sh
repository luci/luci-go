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

# Copies some components/auth protos from luci-py, assuming infra.git gclient
# layout. Generates generate.go and *.pb.go.

set -e

MYPATH=$(dirname "${BASH_SOURCE[0]}")
cd ${MYPATH}

LUCI_PY=../../../../../../../../luci
LUCI_PY_PROTOS_DIR=${LUCI_PY}/appengine/components/components/auth/proto

# Kill all existing files.
rm -rf components
mkdir -p components/auth/proto
rm -f generate.go

# Copy fresh files.
cp \
  ${LUCI_PY_PROTOS_DIR}/realms.proto \
  ${LUCI_PY_PROTOS_DIR}/replication.proto \
  ${LUCI_PY_PROTOS_DIR}/security_config.proto \
  \
  components/auth/proto

# Make proto import paths relative to the new root.
sed -i '' 's|import "components/auth/proto/|import "go.chromium.org/luci/server/auth/service/protocol/components/auth/proto/|g' components/auth/proto/*.proto

# Put the revision of copied files into generate.go for posterity.
luci_py_rev=$(git -C ${LUCI_PY} rev-parse HEAD)

cat <<EOF >> generate.go
// Copyright 2022 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:generate cproto -disable-grpc components/auth/proto

package protocol

// Files copied from https://chromium.googlesource.com/infra/luci/luci-py:
//   appengine/components/components/auth/proto/realms.proto
//   appengine/components/components/auth/proto/replication.proto
//   appengine/components/components/auth/proto/security_config.proto
//
// Commit: ${luci_py_rev}
// Modifications: see import_luci_py.sh
EOF

# Generate *.pb.go.
go generate .
