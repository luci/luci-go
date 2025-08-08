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
	"fmt"
	"testing"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
)

func TestIDConversion(t *testing.T) {
	ftt.Run("ID", t, func(t *ftt.Test) {
		id := ID{
			RootInvocationID: "root-inv-id",
			WorkUnitID:       "work-unit-id",
		}
		t.Run(`Key`, func(t *ftt.Test) {
			expectedKey := spanner.Key{"8d2c0941:root-inv-id", "work-unit-id"}
			assert.That(t, id.Key(), should.Match(expectedKey))

			id2 := ID{
				RootInvocationID: "build123",
				WorkUnitID:       "swarming213:1234567890",
			}
			expectedKey = spanner.Key{"05c33bcc:build123", "swarming213:1234567890"}
			assert.That(t, id2.Key(), should.Match(expectedKey))

			id3 := ID{
				RootInvocationID: "ants-123",
				WorkUnitID:       "root",
			}
			expectedKey = spanner.Key{"43930c7a:ants-123", "root"}
			assert.That(t, id3.Key(), should.Match(expectedKey))
		})

		t.Run(`ShardID`, func(t *ftt.Test) {
			assert.Loosely(t, ID{RootInvocationID: "root-inv-id", WorkUnitID: "work-unit-id"}.shardID(10), should.Equal(8))
			assert.Loosely(t, ID{RootInvocationID: "build123", WorkUnitID: "swarming213:1234567890"}.shardID(10), should.Equal(8))
			assert.Loosely(t, ID{RootInvocationID: "ants-1234", WorkUnitID: "root"}.shardID(10), should.Equal(0))
			assert.Loosely(t, ID{RootInvocationID: "root-inv-id", WorkUnitID: "work-unit-id"}.shardID(150), should.Equal(58))
		})

		t.Run(`LegacyInvocationID`, func(t *ftt.Test) {
			t.Run(`prefixed`, func(t *ftt.Test) {
				prefixedID := ID{
					RootInvocationID: "root-inv-id",
					WorkUnitID:       "some-prefix:work-unit-id",
				}
				assert.Loosely(t, prefixedID.LegacyInvocationID(), should.Equal("workunit:root-inv-id:some-prefix:work-unit-id"))
			})
			t.Run(`non-prefixed`, func(t *ftt.Test) {
				id := ID{
					RootInvocationID: "root-inv-id",
					WorkUnitID:       "work-unit-id",
				}
				assert.Loosely(t, id.LegacyInvocationID(), should.Equal("workunit:root-inv-id:work-unit-id"))
			})
		})

		t.Run(`MustParseLegacyInvocationID`, func(t *ftt.Test) {
			t.Run(`prefixed`, func(t *ftt.Test) {
				legacyID := invocations.ID("workunit:root-inv-id:some-prefix:work-unit-id")
				expectedID := ID{
					RootInvocationID: "root-inv-id",
					WorkUnitID:       "some-prefix:work-unit-id",
				}
				assert.Loosely(t, MustParseLegacyInvocationID(legacyID), should.Equal(expectedID))
			})
			t.Run(`non-prefixed`, func(t *ftt.Test) {
				legacyID := invocations.ID("workunit:root-inv-id:work-unit-id")
				expectedID := ID{
					RootInvocationID: "root-inv-id",
					WorkUnitID:       "work-unit-id",
				}
				assert.Loosely(t, MustParseLegacyInvocationID(legacyID), should.Equal(expectedID))
			})
		})

		t.Run(`RootInvocationShardID`, func(t *ftt.Test) {
			assert.That(t, id.RootInvocationShardID(), should.Equal(rootinvocations.ShardID{RootInvocationID: "root-inv-id", ShardIndex: 8}))
		})

		t.Run(`Name`, func(t *ftt.Test) {
			assert.That(t, id.Name(), should.Equal("rootInvocations/root-inv-id/workUnits/work-unit-id"))
		})

		t.Run(`IDFromRowID`, func(t *ftt.Test) {
			id := IDFromRowID("fd2c0941:root-inv-id", "work-unit-id")
			assert.That(t, id, should.Match(id))
		})

		t.Run(`MustParseName`, func(t *ftt.Test) {
			t.Run(`Valid`, func(t *ftt.Test) {
				assert.That(t, MustParseName("rootInvocations/root-inv-id/workUnits/work-unit-id"), should.Match(id))
				assert.That(t, MustParseName("rootInvocations/build123/workUnits/swarming213:a"), should.Match(ID{RootInvocationID: "build123", WorkUnitID: "swarming213:a"}))
			})
			t.Run(`Invalid`, func(t *ftt.Test) {
				assert.Loosely(t, func() { MustParseName("rootInvocations/root-inv-id/workUnits") }, should.Panic)
			})
		})
	})

	ftt.Run("IDSet", t, func(t *ftt.Test) {
		id1 := ID{RootInvocationID: "a", WorkUnitID: "1"}
		id2 := ID{RootInvocationID: "b", WorkUnitID: "2"}
		id3 := ID{RootInvocationID: "c", WorkUnitID: "3"}
		id4 := ID{RootInvocationID: "c", WorkUnitID: "4"}
		id5 := ID{RootInvocationID: "d", WorkUnitID: "2"}
		s := NewIDSet(id1, id2, id3, id4, id5)
		t.Run("RemoveAll", func(t *ftt.Test) {
			t.Run("Empty", func(t *ftt.Test) {
				emptySet := NewIDSet()
				emptySet.RemoveAll(s)
				assert.Loosely(t, len(emptySet), should.Equal(0))
			})
			t.Run("Non-empty", func(t *ftt.Test) {
				other := NewIDSet(id1, id3)
				s.RemoveAll(other)
				assert.Loosely(t, len(s), should.Equal(3))
				assert.Loosely(t, s.Has(id1), should.BeFalse)
				assert.Loosely(t, s.Has(id2), should.BeTrue)
				assert.Loosely(t, s.Has(id3), should.BeFalse)
				assert.Loosely(t, s.Has(id4), should.BeTrue)
				assert.Loosely(t, s.Has(id5), should.BeTrue)
			})
		})
		t.Run("SortedByID", func(t *ftt.Test) {
			t.Run("Empty", func(t *ftt.Test) {
				emptySet := NewIDSet()
				emptySlice := emptySet.SortedByID()
				assert.Loosely(t, len(emptySlice), should.Equal(0))
			})
			t.Run("Non-empty", func(t *ftt.Test) {
				s := NewIDSet(id1, id2, id3, id4, id5)
				slice := s.SortedByID()
				assert.Loosely(t, len(slice), should.Equal(5))
				assert.Loosely(t, slice[0], should.Equal(id1))
				assert.Loosely(t, slice[1], should.Equal(id2))
				assert.Loosely(t, slice[2], should.Equal(id3))
				assert.Loosely(t, slice[3], should.Equal(id4))
				assert.Loosely(t, slice[4], should.Equal(id5))
			})
		})
		t.Run("SortedByRowID", func(t *ftt.Test) {
			t.Run("Empty", func(t *ftt.Test) {
				emptySet := NewIDSet()
				emptySlice := emptySet.SortedByRowID()
				assert.Loosely(t, len(emptySlice), should.Equal(0))
			})
			t.Run("Non-empty", func(t *ftt.Test) {
				s := NewIDSet(id1, id2, id3, id4, id5)
				slice := s.SortedByRowID()

				// Check the Row IDs are in order.
				rowIDs := make([]string, len(slice))
				for i, id := range slice {
					rowIDs[i] = fmt.Sprintf("%s,%s", id.RootInvocationShardID().RowID(), id.WorkUnitID)
				}
				assert.Loosely(t, rowIDs, should.Match([]string{
					"58ac3e73:d,2",
					"6a978112:a,1",
					"7e23e816:b,2",
					"7e7d2c03:c,4",
					"be7d2c03:c,3",
				}))
			})
		})
	})
}
