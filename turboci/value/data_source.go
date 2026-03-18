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
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// DataSource is an abstraction over a map which retains digest->ValueData.
//
// See [SyncDataSource] and [SimpleDataSource].
type DataSource interface {
	// Retrieve returns the data associated with this digest, or nil if it is
	// absent.
	Retrieve(digest string) *orchestratorpb.ValueData

	// Intern ingests all data from the given map into this DataSource.
	//
	// This must prefer JSON over binary (so interning binary data to an entry
	// with JSON in it will always keep the JSON data).
	//
	// A set conversion failure must override an unset conversion failure.
	Intern(data map[string]*orchestratorpb.ValueData)
}

// MergeData returns a shallow copy of `a` with `b` merged into it using the
// rules defined in [DataSource.Intern].
//
// If a is nil, returns b.
// If merging b into a is a no-op, returns `a` unmodified.
func MergeData(a, b *orchestratorpb.ValueData) *orchestratorpb.ValueData {
	if a == nil {
		return b
	}

	var ret *orchestratorpb.ValueData
	cow := func() *orchestratorpb.ValueData {
		if ret == nil {
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
		cow().SetJson(b.GetJson())
	}
	if !a.HasConversionFailure() && b.HasConversionFailure() {
		cow().SetConversionFailure(b.GetConversionFailure())
	}

	if ret == nil {
		ret = a
	}
	return ret
}
