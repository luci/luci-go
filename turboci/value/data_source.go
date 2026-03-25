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
	"google.golang.org/protobuf/proto"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// DataSource is an abstraction over a map which retains digest->ValueData.
//
// See [SyncDataSource] and [SimpleDataSource].
type DataSource interface {
	// Retrieve returns the data associated with this digest, or nil if it is
	// absent.
	Retrieve(digest Digest) *orchestratorpb.ValueData

	// Intern ingests one digest/data pair.
	//
	// For existing values, use [MergeData] to merge `data` with the existing
	// value.
	//
	// A set conversion failure must override an unset conversion failure.
	Intern(digest Digest, data *orchestratorpb.ValueData)

	// UpdateFrom merges all data from `data` into this DataSource.
	//
	// Useful when getting `value_data` back from read APIs.
	UpdateFrom(data map[string]*orchestratorpb.ValueData)
}

// MergeData returns a shallow copy of `a` with `b` merged into it using the
// rules defined in [DataSource.Intern].
//
// If a is nil, returns b.
// If merging b into a is a no-op, returns `a` unmodified.
//
// Returns the size delta of replacing `a` with `b`.
func MergeData(a, b *orchestratorpb.ValueData) (int64, *orchestratorpb.ValueData) {
	if a == nil {
		return int64(proto.Size(b)), b
	}

	var ret *orchestratorpb.ValueData
	cow := func() *orchestratorpb.ValueData {
		if ret == nil {
			// Shallow copy avoids duplicating binary.value which is `[]byte`.
			ret = &orchestratorpb.ValueData{}
			if a.HasBinary() {
				ret.SetBinary(a.GetBinary())
			} else {
				ret.SetJson(a.GetJson())
			}
			if a.HasConversionFailure() {
				ret.SetConversionFailure(a.GetConversionFailure())
			}
		}
		return ret
	}

	if a.HasBinary() && b.HasJson() {
		mod := cow()
		mod.SetJson(b.GetJson())
		mod.ClearConversionFailure()
	} else if !a.HasConversionFailure() && b.HasConversionFailure() {
		cow().SetConversionFailure(b.GetConversionFailure())
	}

	var delta int64
	if ret == nil {
		ret = a
	} else {
		delta = int64(proto.Size(ret) - proto.Size(a))
	}
	return delta, ret
}
