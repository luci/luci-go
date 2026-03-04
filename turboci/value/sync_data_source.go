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

package value

import (
	"sync"
	"sync/atomic"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// SyncDataSource implements [DataSource] using per-digest synchronization.
//
// Prefer this implementation if you have multiple concurrent readers or
// writers for this DataSource. Otherwise, consider [SimpleDataSource],
type SyncDataSource struct {
	sizeHint atomic.Int64
	data     sync.Map
}

var _ DataSource = (*SyncDataSource)(nil)

// Retrieve implements [DataSource].
func (s *SyncDataSource) Retrieve(digest string) *orchestratorpb.ValueData {
	got, loaded := s.data.Load(digest)
	if loaded {
		return got.(*orchestratorpb.ValueData)
	}
	return nil
}

// Intern implements [DataSource].
func (s *SyncDataSource) Intern(data map[string]*orchestratorpb.ValueData) {
	for digest, dat := range data {
		curVal, loaded := s.data.Load(digest)
		if loaded {
			cur := curVal.(*orchestratorpb.ValueData)
			if cur.HasBinary() && dat.HasJson() {
				s.data.CompareAndSwap(digest, cur, dat)
			}
		} else {
			if _, loaded := s.data.LoadOrStore(digest, dat); !loaded {
				s.sizeHint.Add(1)
			}
		}
	}
}

// ToMap scans all the data in this SyncDataSource and returns it as a
// ValueData map.
func (m *SyncDataSource) ToMap() map[string]*orchestratorpb.ValueData {
	ret := make(map[string]*orchestratorpb.ValueData, m.sizeHint.Load())
	for key, val := range m.data.Range {
		ret[key.(string)] = val.(*orchestratorpb.ValueData)
	}
	return ret
}
