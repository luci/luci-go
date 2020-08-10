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

	"go.chromium.org/luci/server/tq/internal/db"
	"go.chromium.org/luci/server/tq/internal/partition"
	"go.chromium.org/luci/server/tq/internal/reminder"
	"go.chromium.org/luci/server/tq/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestScan(t *testing.T) {
	t.Parallel()

	const keySpaceBytes = 16

	Convey("Scan works", t, func() {
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

		Convey("Normal operation", func() {
			Convey("Empty", func() {
				rems, more := scan(ctx, part0to10)
				So(rems, ShouldBeEmpty)
				So(more, ShouldBeEmpty)
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
				So(db.SaveReminder(ctx, r), ShouldBeNil)
			}
			tclock.Set(epoch.Add(60 * time.Second))

			Convey("Scan complete partition", func() {
				tasksPerScan = 10

				rems, more := scan(ctx, part0to10)
				So(more, ShouldBeEmpty)
				So(rems, ShouldResemble, []*reminder.Reminder{
					mkReminder(2, stale),
					mkReminder(4, stale),
				})

				Convey("but only within given partition", func() {
					rems, more := scan(ctx, partition.FromInts(0, 4))
					So(more, ShouldBeEmpty)
					So(rems, ShouldResemble, []*reminder.Reminder{mkReminder(2, stale)})
				})
			})

			Convey("Scan reaches TasksPerScan limit", func() {
				// Only Reminders 01..03 will be fetched. 02 is stale.
				// Follow up scans should start from 04.
				tasksPerScan = 3
				secondaryScanShards = 2

				rems, more := scan(ctx, part0to10)

				So(rems, ShouldResemble, []*reminder.Reminder{mkReminder(2, stale)})

				Convey("Scan returns correct follow up ScanItems", func() {
					So(len(more), ShouldEqual, secondaryScanShards)
					So(more[0].Low, ShouldResemble, *big.NewInt(3 + 1))
					So(more[0].High, ShouldResemble, more[1].Low)
					So(more[1].High, ShouldResemble, *big.NewInt(10))
				})
			})
		})

		Convey("Abnormal operation", func() {
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
			So(ctx.Err(), ShouldResemble, context.DeadlineExceeded)

			Convey("Timeout without anything fetched", func() {
				mockDB.EXPECT().FetchRemindersMeta(ctx, gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.Annotate(ctx.Err(), "failed to fetch all").Err())

				rems, more := scan(ctx, part0to10)

				So(rems, ShouldBeEmpty)
				So(len(more), ShouldEqual, secondaryScanShards)
				So(more[0].Low, ShouldResemble, *big.NewInt(0))
				So(more[1].High, ShouldResemble, *big.NewInt(10))
			})

			Convey("Timeout after fetching some", func() {
				mockDB.EXPECT().FetchRemindersMeta(ctx, gomock.Any(), gomock.Any(), gomock.Any()).
					Return(someReminders, ctx.Err())

				rems, more := scan(ctx, part0to10)

				So(rems, ShouldResemble, []*reminder.Reminder{mkReminder(2, stale)})
				So(more, ShouldHaveLength, 1) // partition is so small, only 1 follow up scan suffices.
				So(more[0].Low, ShouldResemble, *big.NewInt(2 + 1))
				So(more[0].High, ShouldResemble, *big.NewInt(10))
			})
		})
	})
}
