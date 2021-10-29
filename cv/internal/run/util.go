// Copyright 2021 The LUCI Authors.
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

package run

import "go.chromium.org/luci/gae/service/datastore"

// runHeapKey facilitates heap-based merge of multiple consistently sorted
// ranges of Datastore keys, each range identified by its index.
type runHeapKey struct {
	dsKey   *datastore.Key
	sortKey string
	idx     int
}
type runHeap []runHeapKey

func (r runHeap) Len() int {
	return len(r)
}

func (r runHeap) Less(i int, j int) bool {
	return r[i].sortKey < r[j].sortKey
}

func (r runHeap) Swap(i int, j int) {
	r[i], r[j] = r[j], r[i]
}

func (r *runHeap) Push(x interface{}) {
	*r = append(*r, x.(runHeapKey))
}

func (r *runHeap) Pop() interface{} {
	idx := len(*r) - 1
	v := (*r)[idx]
	(*r)[idx].dsKey = nil // free memory as a good habit.
	*r = (*r)[:idx]
	return v
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}
