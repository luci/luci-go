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
// Directly construct either via allocation, or by casting a compatible
// map type.
//
// If you need a DataSource to persist through multiple calls, or where you
// need multiple readers or writers, consider [SyncDataSource].
type SimpleDataSource map[string]*orchestratorpb.ValueData

var _ DataSource = SimpleDataSource(nil)

// Retrieve implements [DataSource].
func (s SimpleDataSource) Retrieve(digest Digest) *orchestratorpb.ValueData {
	return s[string(digest)]
}

// InternOne implements [DataSource].
func (s SimpleDataSource) Intern(digest Digest, data *orchestratorpb.ValueData) {
	digestS := string(digest)
	_, s[digestS] = MergeData(s[digestS], data)
}

// UpdateFrom implements [DataSource].
func (s SimpleDataSource) UpdateFrom(data map[string]*orchestratorpb.ValueData) {
	for digest, dat := range data {
		_, s[digest] = MergeData(s[digest], dat)
	}
}
