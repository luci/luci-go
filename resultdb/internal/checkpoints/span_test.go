// Copyright 2024 The LUCI Authors.
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

package checkpoints

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/testutil"
)

func TestCheckpoints(t *testing.T) {
	ftt.Run("With Spanner context", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		now := time.Date(2055, time.May, 5, 5, 5, 5, 5, time.UTC)
		ctx, _ = testclock.UseTime(ctx, now)

		t.Run("Exists", func(t *ftt.Test) {
			t.Run("Does not exist", func(t *ftt.Test) {
				key := Key{
					Project:    "project",
					ResourceID: "resource-id",
					ProcessID:  "process-id",
					Uniquifier: "uniqifier",
				}
				exists, err := Exists(span.Single(ctx), key)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, exists, should.BeFalse)
			})
			t.Run("Exists", func(t *ftt.Test) {
				key := Key{
					Project:    "project",
					ResourceID: "resource-id",
					ProcessID:  "process-id",
					Uniquifier: "uniqifier",
				}
				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					ms := Insert(ctx, key, time.Hour)
					span.BufferWrite(ctx, ms)
					return nil
				})
				assert.Loosely(t, err, should.BeNil)

				exists, err := Exists(span.Single(ctx), key)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, exists, should.BeTrue)
			})
		})
		t.Run("Insert", func(t *ftt.Test) {
			key := Key{
				Project:    "project",
				ResourceID: "resource-id",
				ProcessID:  "process-id",
				Uniquifier: "uniqifier",
			}
			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				ms := Insert(ctx, key, time.Hour)
				span.BufferWrite(ctx, ms)
				return nil
			})
			assert.Loosely(t, err, should.BeNil)

			checkpoints, err := ReadAllForTesting(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, checkpoints, should.HaveLength(1))
			assert.Loosely(t, checkpoints[0].Key, should.Match(key))
			assert.Loosely(t, checkpoints[0].CreationTime, should.NotBeZero)
			assert.Loosely(t, checkpoints[0].ExpiryTime, should.Match(now.Add(time.Hour)))
		})
		t.Run("ReadAllUniquifiers", func(t *ftt.Test) {
			key1 := Key{
				Project:    "project",
				ResourceID: "resource-id",
				ProcessID:  "process-id",
				Uniquifier: "uniqifier1",
			}
			key2 := Key{
				Project:    "project",
				ResourceID: "resource-id",
				ProcessID:  "process-id",
				Uniquifier: "uniqifier2",
			}

			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				ms1 := Insert(ctx, key1, time.Hour)
				ms2 := Insert(ctx, key2, time.Hour)
				span.BufferWrite(ctx, ms1, ms2)
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
			uqs, err := ReadAllUniquifiers(span.Single(ctx), "project", "resource-id", "process-id")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, uqs, should.Match(map[string]bool{
				"uniqifier1": true,
				"uniqifier2": true,
			}))
		})
	})
}
