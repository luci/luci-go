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

	"google.golang.org/protobuf/proto"

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
	dataSize atomic.Int64
	data     sync.Map
}

var _ DataSource = (*SyncDataSource)(nil)

// SyncDataSourceFromMap constructs a SyncDataSource initialized with the
// data in `valueData`.
func SyncDataSourceFromMap(valueData map[string]*orchestratorpb.ValueData) *SyncDataSource {
	ret := &SyncDataSource{}
	dataSize := int64(0)
	for digest, data := range valueData {
		ret.data.Store(Digest(digest), data)
		dataSize += int64(len(digest) + proto.Size(data))
	}
	ret.sizeHint.Store(int64(len(valueData)))
	ret.dataSize.Store(dataSize)
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
//
// This will not atomically update DataSize; If there are multiple goroutines
// calling Intern and also DataSize in parallel, you will see an inconsistent
// value. If you have this pattern, you need some external synchronization
// for your calls to Intern and DataSize.
func (s *SyncDataSource) Intern(digest Digest, data *orchestratorpb.ValueData) {
	if curVal, loaded := s.data.LoadOrStore(digest, data); loaded {
		cur := curVal.(*orchestratorpb.ValueData)
		delta, newDat := MergeData(cur, data)
		for cur != newDat && !s.data.CompareAndSwap(digest, cur, newDat) {
			curVal, loaded = s.data.Load(digest)
			if !loaded {
				panic("impossible")
			}
			cur = curVal.(*orchestratorpb.ValueData)
			delta, newDat = MergeData(cur, data)
		}
		s.dataSize.Add(delta)
	} else {
		s.dataSize.Add(int64(proto.Size(data) + len(digest)))
		s.sizeHint.Add(1)
	}
}

// DataSize returns the amount of data in bytes currently in this
// SyncDataSource.
//
// This accounts for the size of the digests as well as the size of the
// ValueData.
func (s *SyncDataSource) DataSize() int64 {
	return s.dataSize.Load()
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
