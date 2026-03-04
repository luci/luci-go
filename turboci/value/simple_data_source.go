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

// SimpleDataSource implements the simplest possible [DataSource] with no
// synchronization.
//
// Useful to transform a ValueData map from an RPC response into a [DataSource]
// without any extra allocation or overhead.
//
// If you need a DataSource to persist through multiple calls, or where you
// need multiple readers or writers, consider [SyncDataSource].
type SimpleDataSource map[string]*orchestratorpb.ValueData

var _ DataSource = SimpleDataSource(nil)

// Retrieve implements [DataSource].
func (s SimpleDataSource) Retrieve(digest string) *orchestratorpb.ValueData {
	return s[digest]
}

// Intern implements [DataSource].
func (s SimpleDataSource) Intern(data map[string]*orchestratorpb.ValueData) {
	for digest, dat := range data {
		if cur, ok := s[digest]; ok {
			if cur.HasBinary() && dat.HasJson() {
				s[digest] = dat
			}
		} else {
			s[digest] = dat
		}
	}
}
