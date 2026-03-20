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
//
// Construct with simple allocation, e.g. `&SyncDataSource{}` or via
// [SyncDataSourceFromMap].
type SyncDataSource struct {
	sizeHint atomic.Int64
	data     sync.Map
}

var _ DataSource = (*SyncDataSource)(nil)

// SyncDataSourceFromMap constructs a SyncDataSource initialized with the
// data in `valueData`.
func SyncDataSourceFromMap(valueData map[string]*orchestratorpb.ValueData) *SyncDataSource {
	ret := &SyncDataSource{}
	for digest, data := range valueData {
		ret.data.Store(Digest(digest), data)
	}
	ret.sizeHint.Add(int64(len(valueData)))
	return ret
}

// Retrieve implements [DataSource].
func (s *SyncDataSource) Retrieve(digest Digest) *orchestratorpb.ValueData {
	got, loaded := s.data.Load(digest)
	if loaded {
		return got.(*orchestratorpb.ValueData)
	}
	return nil
}

// Intern implements [DataSource].
func (s *SyncDataSource) Intern(digest Digest, data *orchestratorpb.ValueData) {
	if curVal, loaded := s.data.LoadOrStore(digest, data); loaded {
		cur := curVal.(*orchestratorpb.ValueData)
		newDat := MergeData(cur, data)
		for cur != newDat && !s.data.CompareAndSwap(digest, cur, newDat) {
			curVal, loaded = s.data.Load(digest)
			if !loaded {
				panic("impossible")
			}
			cur = curVal.(*orchestratorpb.ValueData)
			newDat = MergeData(cur, data)
		}
	} else {
		s.sizeHint.Add(1)
	}
}

func (s *SyncDataSource) UpdateFrom(data map[string]*orchestratorpb.ValueData) {
	for digest, dat := range data {
		s.Intern(Digest(digest), dat)
	}
}

// ToMap scans all the data in this SyncDataSource and returns it as a
// ValueData map.
func (s *SyncDataSource) ToMap() map[string]*orchestratorpb.ValueData {
	ret := make(map[string]*orchestratorpb.ValueData, s.sizeHint.Load())
	for key, val := range s.data.Range {
		ret[string(key.(Digest))] = val.(*orchestratorpb.ValueData)
	}
	return ret
}
