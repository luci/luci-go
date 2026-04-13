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
	Retrieve(digest Digest) *orchestratorpb.ValueData

	// Intern ingests one digest/data pair.
	//
	// For existing values, use [PickData] to merge `data` with the existing
	// value.
	//
	// A set conversion failure must override an unset conversion failure.
	Intern(digest Digest, data *orchestratorpb.ValueData)

	// UpdateFrom merges all data from `data` into this DataSource.
	//
	// Useful when getting `value_data` back from read APIs.
	UpdateFrom(data map[string]*orchestratorpb.ValueData)
}

// PickData returns either `a` or `b` depending on which is better.
//
// Both `a` and `b` must be well-formed (one of `binary` or `json` must be
// populated)
//
// Prefers JSON without unknown fields to JSON with unknown fields.
// Prefers JSON to binary data.
// Prefers binary data with conversion_failure enum to binary data without.
func PickData(a, b *orchestratorpb.ValueData) *orchestratorpb.ValueData {
	if a == nil {
		return b
	}

	if a.HasBinary() && b.HasJson() {
		return b
	}

	if a.HasJson() && b.HasJson() {
		if a.GetJson().GetHasUnknownFields() && !b.GetJson().GetHasUnknownFields() {
			return b
		}
	}

	if a.HasJson() && !b.HasJson() {
		return a
	}

	if !a.HasConversionFailure() && b.HasConversionFailure() {
		return b
	}

	return a
}
