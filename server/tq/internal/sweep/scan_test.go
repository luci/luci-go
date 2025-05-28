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

package sweep

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/golang/mock/gomock"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/tq/internal/db"
	"go.chromium.org/luci/server/tq/internal/partition"
	"go.chromium.org/luci/server/tq/internal/reminder"
	"go.chromium.org/luci/server/tq/internal/testutil"
)

func TestScan(t *testing.T) {
	t.Parallel()

	const keySpaceBytes = 16

	ftt.Run("Scan works", t, func(t *ftt.Test) {
		epoch := testclock.TestRecentTimeLocal
		ctx, tclock := testclock.UseTime(context.Background(), epoch)
		db := db.DB(&testutil.FakeDB{})

		mkReminder := func(id int64, freshUntil time.Time) *reminder.Reminder {
			low, _ := partition.FromInts(id, id+1).QueryBounds(keySpaceBytes)
			return &reminder.Reminder{ID: low, FreshUntil: freshUntil}
		}
		part0to10 := partition.FromInts(0, 10)

		tasksPerScan := 100
		secondaryScanShards := 16

		scan := func(ctx context.Context, part *partition.Partition) ([]*reminder.Reminder, partition.SortedPartitions) {
			return Scan(ctx, &ScanParams{
				DB:                  db,
				Partition:           part,
				KeySpaceBytes:       keySpaceBytes,
				TasksPerScan:        tasksPerScan,
				SecondaryScanShards: secondaryScanShards,
			})
		}

		t.Run("Normal operation", func(t *ftt.Test) {
			t.Run("Empty", func(t *ftt.Test) {
				rems, more := scan(ctx, part0to10)
				assert.Loosely(t, rems, should.BeEmpty)
				assert.Loosely(t, more, should.BeEmpty)
			})

			stale := epoch.Add(30 * time.Second)
			fresh := epoch.Add(90 * time.Second)
			savedReminders := []*reminder.Reminder{
				mkReminder(1, fresh),
				mkReminder(2, stale),
				mkReminder(3, fresh),
				mkReminder(4, stale),
			}
			for _, r := range savedReminders {
				assert.Loosely(t, db.SaveReminder(ctx, r), should.BeNil)
			}
			tclock.Set(epoch.Add(60 * time.Second))

			t.Run("Scan complete partition", func(t *ftt.Test) {
				tasksPerScan = 10

				rems, more := scan(ctx, part0to10)
				assert.Loosely(t, more, should.BeEmpty)
				assert.Loosely(t, rems, should.Resemble([]*reminder.Reminder{
					mkReminder(2, stale),
					mkReminder(4, stale),
				}))

				t.Run("but only within given partition", func(t *ftt.Test) {
					rems, more := scan(ctx, partition.FromInts(0, 4))
					assert.Loosely(t, more, should.BeEmpty)
					assert.Loosely(t, rems, should.Resemble([]*reminder.Reminder{mkReminder(2, stale)}))
				})
			})

			t.Run("Scan reaches TasksPerScan limit", func(t *ftt.Test) {
				// Only Reminders 01..03 will be fetched. 02 is stale.
				// Follow up scans should start from 04.
				tasksPerScan = 3
				secondaryScanShards = 2

				rems, more := scan(ctx, part0to10)

				assert.Loosely(t, rems, should.Resemble([]*reminder.Reminder{mkReminder(2, stale)}))

				t.Run("Scan returns correct follow up ScanItems", func(t *ftt.Test) {
					assert.Loosely(t, len(more), should.Equal(secondaryScanShards))
					assert.Loosely(t, more[0].Low, should.Resemble(*big.NewInt(3 + 1)))
					assert.Loosely(t, more[0].High, should.Resemble(more[1].Low))
					assert.Loosely(t, more[1].High, should.Resemble(*big.NewInt(10)))
				})
			})
		})

		t.Run("Abnormal operation", func(t *ftt.Test) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockDB := testutil.NewMockDB(ctrl)
			mockDB.EXPECT().Kind().AnyTimes().Return("mockdb")

			db = mockDB
			tasksPerScan = 2048
			secondaryScanShards = 2

			stale := epoch.Add(30 * time.Second)
			fresh := epoch.Add(90 * time.Second)
			someReminders := []*reminder.Reminder{
				mkReminder(1, fresh),
				mkReminder(2, stale),
			}
			tclock.Set(epoch.Add(60 * time.Second))

			// Simulate context expiry by using deadline in the past.
			// TODO(tandrii): find a way to expire ctx within FetchRemindersMeta for a
			// realistic test.
			ctx, cancel := clock.WithDeadline(ctx, epoch)
			defer cancel()
			assert.Loosely(t, ctx.Err(), should.Resemble(context.DeadlineExceeded))

			t.Run("Timeout without anything fetched", func(t *ftt.Test) {
				mockDB.EXPECT().FetchRemindersMeta(ctx, gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.WrapIf(ctx.Err(), "failed to fetch all"))

				rems, more := scan(ctx, part0to10)

				assert.Loosely(t, rems, should.BeEmpty)
				assert.Loosely(t, len(more), should.Equal(secondaryScanShards))
				assert.Loosely(t, more[0].Low, should.Resemble(*big.NewInt(0)))
				assert.Loosely(t, more[1].High, should.Resemble(*big.NewInt(10)))
			})

			t.Run("Timeout after fetching some", func(t *ftt.Test) {
				mockDB.EXPECT().FetchRemindersMeta(ctx, gomock.Any(), gomock.Any(), gomock.Any()).
					Return(someReminders, ctx.Err())

				rems, more := scan(ctx, part0to10)

				assert.Loosely(t, rems, should.Resemble([]*reminder.Reminder{mkReminder(2, stale)}))
				assert.Loosely(t, more, should.HaveLength(1)) // partition is so small, only 1 follow up scan suffices.
				assert.Loosely(t, more[0].Low, should.Resemble(*big.NewInt(2 + 1)))
				assert.Loosely(t, more[0].High, should.Resemble(*big.NewInt(10)))
			})
		})
	})
}
