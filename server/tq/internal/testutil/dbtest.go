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

	"go.chromium.org/luci/server/tq/internal/db"
	"go.chromium.org/luci/server/tq/internal/partition"
	"go.chromium.org/luci/server/tq/internal/reminder"

	. "github.com/smartystreets/goconvey/convey"
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

	epoch := time.Date(2020, time.February, 3, 4, 5, 6, 0, time.UTC)

	// Test uses keySpaceBytes=1, meaning [0..256) keyspace.
	mkReminder := func(id int64, freshUntil time.Time, payload, extra string) *reminder.Reminder {
		var payloadBytes []byte
		if payload != "" { // prefer nil instead of []byte{}
			payloadBytes = []byte(payload)
		}
		var extraBytes []byte
		if extra != "" { // prefer nil instead of []byte{}
			extraBytes = []byte(extra)
		}
		low, _ := partition.FromInts(id, id+1).QueryBounds(1)
		return &reminder.Reminder{
			ID:         low,
			FreshUntil: freshUntil,
			Payload:    payloadBytes,
			Extra:      extraBytes,
		}
	}

	Convey("Test DB "+db.Kind(), t, func() {
		Convey("Save & Delete", func() {
			r := mkReminder(1, epoch, "payload", "extra")
			So(db.SaveReminder(ctx, r), ShouldBeNil)
			So(db.DeleteReminder(ctx, r), ShouldBeNil)
			Convey("Delete non-existing is a noop", func() {
				So(db.DeleteReminder(ctx, r), ShouldBeNil)
			})
		})

		Convey("FetchRemindersMeta", func() {
			So(db.SaveReminder(ctx, mkReminder(254, epoch.Add(time.Second), "254", "ex 1")), ShouldBeNil)
			So(db.SaveReminder(ctx, mkReminder(100, epoch.Add(time.Minute), "pay", "ex 2")), ShouldBeNil)
			So(db.SaveReminder(ctx, mkReminder(255, epoch.Add(time.Hour), "load", "ex 3")), ShouldBeNil)

			Convey("All + sorted", func() {
				res, err := db.FetchRemindersMeta(ctx, "00", "g", 5)
				So(err, ShouldBeNil)
				So(res, ShouldResemble, []*reminder.Reminder{
					mkReminder(100, epoch.Add(time.Minute), "", ""),
					mkReminder(254, epoch.Add(time.Second), "", ""),
					mkReminder(255, epoch.Add(time.Hour), "", ""),
				})
			})

			Convey("Limit", func() {
				res, err := db.FetchRemindersMeta(ctx, "00", "g", 2)
				So(err, ShouldBeNil)
				So(res, ShouldResemble, []*reminder.Reminder{
					mkReminder(100, epoch.Add(time.Minute), "", ""),
					mkReminder(254, epoch.Add(time.Second), "", ""),
				})
			})

			Convey("Obey partition", func() {
				res, err := db.FetchRemindersMeta(ctx, "00", "ee", 5)
				So(err, ShouldBeNil)
				So(res, ShouldResemble, []*reminder.Reminder{
					mkReminder(100, epoch.Add(time.Minute), "", ""),
				})
			})
		})

		Convey("FetchReminderPayloads", func() {
			all := []*reminder.Reminder{
				mkReminder(100, epoch.Add(time.Minute), "pay", "ex 1"),
				mkReminder(254, epoch.Add(time.Second), "254", "ex 2"),
				mkReminder(255, epoch.Add(time.Hour), "load", "ex 3"),
			}
			meta := make([]*reminder.Reminder, len(all))
			for i, r := range all {
				meta[i] = &reminder.Reminder{ID: r.ID, FreshUntil: r.FreshUntil}
				So(db.SaveReminder(ctx, r), ShouldBeNil)
			}

			Convey("All", func() {
				res, err := db.FetchReminderPayloads(ctx, meta)
				So(err, ShouldBeNil)
				So(res, ShouldResemble, all)
				Convey("Re-use objects", func() {
					So(meta, ShouldResemble, all)
				})
			})

			Convey("Some", func() {
				second := &reminder.Reminder{ID: meta[1].ID, FreshUntil: meta[1].FreshUntil}
				So(db.DeleteReminder(ctx, second), ShouldBeNil)
				res, err := db.FetchReminderPayloads(ctx, meta)
				So(err, ShouldBeNil)
				So(res, ShouldResemble, []*reminder.Reminder{all[0], all[2]})
				Convey("Re-use objects", func() {
					So(meta[0], ShouldResemble, all[0])
					So(meta[2], ShouldResemble, all[2])
				})
			})
		})
	})
}
