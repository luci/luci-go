// Copyright 2015 The LUCI Authors.
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

package model

import (
	"context"
	"sort"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/dm/api/service/v1"
)

// FwdDep describes a 'depends-on' relation between two Attempts. It has a
// reciprocal BackDep as well, which notes the depended-on-by relationship. So:
//
//   Attempt(OTHER_QUEST|2)
//     FwdDep(QUEST|1)
//
//   Attempt(QUEST|1)
//
//   BackDepGroup(QUEST|1)
//     BackDep(OTHER_QUEST|2)
//
// Represents the OTHER_QUEST|2 depending on QUEST|1.
type FwdDep struct {
	// Attempt that this points from.
	Depender *ds.Key `gae:"$parent"`

	// A FwdDep's ID is the Attempt ID that it points to.
	Dependee dm.Attempt_ID `gae:"$id"`

	// This will be used to set a bit in the Attempt (WaitingDepBitmap) when the
	// Dep completes.
	BitIndex uint32

	// ForExecution indicates which Execution added this dependency. This is used
	// for validation of AckFwdDep mutations to ensure that they're operating
	// on an Attempt in the correct state, but can also be used for historical
	// analysis/display.
	ForExecution uint32
}

// Edge produces a edge object which points 'forwards' from the depending
// attempt to the depended-on attempt.
func (f *FwdDep) Edge() *FwdEdge {
	ret := &FwdEdge{To: &f.Dependee, From: &dm.Attempt_ID{}}
	if err := ret.From.SetDMEncoded(f.Depender.StringID()); err != nil {
		panic(err)
	}
	return ret
}

// FwdDepsFromList creates a slice of *FwdDep given an originating base
// Attempt_ID, and a list of dependency Attempts.
func FwdDepsFromList(c context.Context, base *dm.Attempt_ID, list *dm.AttemptList) []*FwdDep {
	from := ds.KeyForObj(c, &Attempt{ID: *base})
	keys := make(sort.StringSlice, 0, len(list.To))
	amt := 0
	for qst, nums := range list.To {
		keys = append(keys, qst)
		amt += len(nums.Nums)
	}
	keys.Sort()
	idx := uint32(0)
	ret := make([]*FwdDep, 0, amt)
	for _, key := range keys {
		for _, num := range list.To[key].Nums {
			dep := &FwdDep{Depender: from}
			dep.Dependee.Quest = key
			dep.Dependee.Id = num
			dep.BitIndex = idx
			idx++
			ret = append(ret, dep)
		}
	}
	return ret
}

// FwdDepKeysFromList makes a list of datastore.Key's that correspond to all
// of the FwdDeps expressed by the <base, list> pair.
func FwdDepKeysFromList(c context.Context, base *dm.Attempt_ID, list *dm.AttemptList) []*ds.Key {
	keys := make(sort.StringSlice, 0, len(list.To))
	amt := 0
	for qst, nums := range list.To {
		keys = append(keys, qst)
		amt += len(nums.Nums)
	}
	keys.Sort()
	ret := make([]*ds.Key, 0, amt)
	for _, key := range keys {
		for _, num := range list.To[key].Nums {
			ret = append(ret, ds.MakeKey(c,
				"Attempt", base.DMEncoded(),
				"FwdDep", dm.NewAttemptID(key, num).DMEncoded()))
		}
	}
	return ret
}
