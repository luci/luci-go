// Copyright 2017 The LUCI Authors.
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

package srcman

import (
	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/data/base128"
	"go.chromium.org/luci/common/proto/milo"
)

type srcManCacheEntry struct {
	_  string `gae:"$kind,SrcManCacheEntry"`
	ID string `gae:"$id"`

	Data []byte `gae:",noindex"`
}

func (s *srcManCacheEntry) Read() (*milo.Manifest, error) {
	ret := &milo.Manifest{}
	return ret, proto.Unmarshal(s.Data, ret)
}

func newSrcManCacheEntry(sha2 []byte) *srcManCacheEntry {
	return &srcManCacheEntry{ID: base128.EncodeToString(sha2)}
}

func (s *srcManCacheEntry) Sha256() ([]byte, error) {
	return base128.DecodeString(s.ID)
}
