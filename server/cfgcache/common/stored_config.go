// Copyright 2026 The LUCI Authors.
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

// Package common has common types needed by Store implementations and cfgcache
// itself.
package cfgcachecommon

import (
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/config"

	cfgcachepb "go.chromium.org/luci/server/cfgcache/proto"
)

// StoredConfig holds a stored config value and its version metadata.
type StoredConfig[StoreMsg proto.Message] struct {
	ConfigSet config.Set
	Meta      *cfgcachepb.Meta
	Config    StoreMsg
}
