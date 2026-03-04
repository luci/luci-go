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
	Intern(data map[string]*orchestratorpb.ValueData)
}
