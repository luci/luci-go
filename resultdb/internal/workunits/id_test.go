// Copyright 2025 The LUCI Authors.
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

package workunits

import (
	"testing"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestIDConversion(t *testing.T) {
	ftt.Run("ID", t, func(t *ftt.Test) {
		id := ID{
			RootInvocationID: "root-inv-id",
			WorkUnitID:       "work-unit-id",
		}
		t.Run(`Key`, func(t *ftt.Test) {
			expectedKey := spanner.Key{"8d2c0941:root-inv-id", "work-unit-id"}
			assert.That(t, id.key(), should.Match(expectedKey))

			id2 := ID{
				RootInvocationID: "build123",
				WorkUnitID:       "swarming213:1234567890",
			}
			expectedKey = spanner.Key{"05c33bcc:build123", "swarming213:1234567890"}
			assert.That(t, id2.key(), should.Match(expectedKey))

			id3 := ID{
				RootInvocationID: "ants-123",
				WorkUnitID:       "root",
			}
			expectedKey = spanner.Key{"43930c7a:ants-123", "root"}
			assert.That(t, id3.key(), should.Match(expectedKey))
		})

		t.Run(`ShardID`, func(t *ftt.Test) {
			assert.Loosely(t, ID{RootInvocationID: "root-inv-id", WorkUnitID: "work-unit-id"}.shardID(10), should.Equal(8))
			assert.Loosely(t, ID{RootInvocationID: "build123", WorkUnitID: "swarming213:1234567890"}.shardID(10), should.Equal(8))
			assert.Loosely(t, ID{RootInvocationID: "ants-1234", WorkUnitID: "root"}.shardID(10), should.Equal(0))
			assert.Loosely(t, ID{RootInvocationID: "root-inv-id", WorkUnitID: "work-unit-id"}.shardID(150), should.Equal(58))
		})

		t.Run(`LegacyInvocationWorkUnitID`, func(t *ftt.Test) {
			assert.Loosely(t, id.LegacyInvocationID(), should.Equal("workunit:root-inv-id:work-unit-id"))
		})

		t.Run(`RootInvocationShardID`, func(t *ftt.Test) {
			assert.That(t, id.rootInvocationShardID(), should.Equal("8d2c0941:root-inv-id"))
		})

		t.Run(`Name`, func(t *ftt.Test) {
			assert.That(t, id.Name(), should.Equal("rootInvocations/root-inv-id/workUnits/work-unit-id"))
		})

		t.Run(`IDFromRowID`, func(t *ftt.Test) {
			id := IDFromRowID("fd2c0941:root-inv-id", "work-unit-id")
			assert.That(t, id, should.Match(id))
		})
	})
}
