// Copyright 2020 The LUCI Authors.
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

package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/tq/internal/db"
	"go.chromium.org/luci/server/tq/internal/partition"
	"go.chromium.org/luci/server/tq/internal/reminder"
)

// RunDBAcceptance tests a database implementation.
//
// Sadly, GoConvey-reported error lines are borked because it doesn't search
// stack in files other than "*_test.go" and "*_tests.go", which this file
// can't be as it is in a different package.
// However, you can run tests for your database via $ go test
// and examine stacktraces for actual true line nimber.
// Or, you can copy this file to your package during debugging.
func RunDBAcceptance(ctx context.Context, db db.DB, t *testing.T) {
	t.Parallel()

	// TODO(gregorynisbet): Put this setup logic in TestMain.
	// Putting this registration logic here works, but putting it in TestMain does not.
	// I don't know why.
	registry.RegisterCmpOption(cmp.AllowUnexported(reminder.Reminder{}))

	epoch := time.Date(2020, time.February, 3, 4, 5, 6, 0, time.UTC)

	// Test uses keySpaceBytes=1, meaning [0..256) keyspace.
	mkReminder := func(id int64, freshUntil time.Time, payload string) *reminder.Reminder {
		var payloadBytes []byte
		if payload != "" { // prefer nil instead of []byte{}
			payloadBytes = []byte(payload)
		}
		low, _ := partition.FromInts(id, id+1).QueryBounds(1)
		return &reminder.Reminder{
			ID:         low,
			FreshUntil: freshUntil,
			RawPayload: payloadBytes,
		}
	}

	ftt.Run("Test DB "+db.Kind(), t, func(t *ftt.Test) {
		t.Run("Save & Delete", func(t *ftt.Test) {
			r := mkReminder(1, epoch, "payload")
			assert.Loosely(t, db.SaveReminder(ctx, r), should.BeNil)
			assert.Loosely(t, db.DeleteReminder(ctx, r), should.BeNil)
			t.Run("Delete non-existing is a noop", func(t *ftt.Test) {
				assert.Loosely(t, db.DeleteReminder(ctx, r), should.BeNil)
			})
		})

		t.Run("FetchRemindersMeta", func(t *ftt.Test) {
			assert.Loosely(t, db.SaveReminder(ctx, mkReminder(254, epoch.Add(time.Second), "254")), should.BeNil)
			assert.Loosely(t, db.SaveReminder(ctx, mkReminder(100, epoch.Add(time.Minute), "pay")), should.BeNil)
			assert.Loosely(t, db.SaveReminder(ctx, mkReminder(255, epoch.Add(time.Hour), "load")), should.BeNil)

			t.Run("All + sorted", func(t *ftt.Test) {
				res, err := db.FetchRemindersMeta(ctx, "00", "g", 5)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match([]*reminder.Reminder{
					mkReminder(100, epoch.Add(time.Minute), ""),
					mkReminder(254, epoch.Add(time.Second), ""),
					mkReminder(255, epoch.Add(time.Hour), ""),
				}))
			})

			t.Run("Limit", func(t *ftt.Test) {
				res, err := db.FetchRemindersMeta(ctx, "00", "g", 2)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match([]*reminder.Reminder{
					mkReminder(100, epoch.Add(time.Minute), ""),
					mkReminder(254, epoch.Add(time.Second), ""),
				}))
			})

			t.Run("Obey partition", func(t *ftt.Test) {
				res, err := db.FetchRemindersMeta(ctx, "00", "ee", 5)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match([]*reminder.Reminder{
					mkReminder(100, epoch.Add(time.Minute), ""),
				}))
			})
		})

		t.Run("FetchReminderRawPayloads", func(t *ftt.Test) {
			all := []*reminder.Reminder{
				mkReminder(100, epoch.Add(time.Minute), "pay"),
				mkReminder(254, epoch.Add(time.Second), "254"),
				mkReminder(255, epoch.Add(time.Hour), "load"),
			}
			meta := make([]*reminder.Reminder, len(all))
			for i, r := range all {
				meta[i] = &reminder.Reminder{ID: r.ID, FreshUntil: r.FreshUntil}
				assert.Loosely(t, db.SaveReminder(ctx, r), should.BeNil)
			}

			t.Run("All", func(t *ftt.Test) {
				res, err := db.FetchReminderRawPayloads(ctx, meta)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(all))
				t.Run("Re-use objects", func(t *ftt.Test) {
					assert.Loosely(t, meta, should.Match(all))
				})
			})

			t.Run("Some", func(t *ftt.Test) {
				second := &reminder.Reminder{ID: meta[1].ID, FreshUntil: meta[1].FreshUntil}
				assert.Loosely(t, db.DeleteReminder(ctx, second), should.BeNil)
				res, err := db.FetchReminderRawPayloads(ctx, meta)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match([]*reminder.Reminder{all[0], all[2]}))
				t.Run("Re-use objects", func(t *ftt.Test) {
					assert.Loosely(t, meta[0], should.Match(all[0]))
					assert.Loosely(t, meta[2], should.Match(all[2]))
				})
			})
		})
	})
}
