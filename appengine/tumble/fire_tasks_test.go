// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tumble

import (
	"math"
	"testing"
	"time"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/taskqueue"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/memlogger"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

var _ = math.E

func TestShardCalculation(t *testing.T) {
	t.Parallel()

	Convey("shard calculation", t, func() {
		numShards := uint64(11)

		ctx := memlogger.Use(context.Background())
		ctx = Use(ctx, Config{NumShards: numShards})

		cfg := GetConfig(ctx)

		l := logging.Get(ctx).(*memlogger.MemLogger)

		Convey("ExpandedShard->Shard covers the full int64 range", func() {
			tcs := []struct {
				es int64
				s  uint64
			}{
				{math.MinInt64, 0},
				{0, numShards / 2},
				{math.MaxInt64, numShards - 1},
			}

			for _, tc := range tcs {
				So((&realMutation{ExpandedShard: tc.es}).shard(&cfg), ShouldEqual, tc.s)
				low, high := expandedShardBounds(ctx, tc.s)
				So(tc.es, ShouldBeGreaterThanOrEqualTo, low)
				So(tc.es, ShouldBeLessThanOrEqualTo, high)
				So(l.Messages(), ShouldBeEmpty)
			}
		})

		Convey("expandedShardsPerShard returns crossed ranges on shard reduction", func() {
			low, high := expandedShardBounds(ctx, 256)
			So(low, ShouldBeGreaterThan, high)
			So(l, memlogger.ShouldHaveLog, logging.Warning, "Invalid shard: 256")
		})
	})
}

func TestFireTasks(t *testing.T) {
	t.Parallel()

	Convey("fireTasks works as expected", t, func() {
		ctx := memory.Use(context.Background())
		ctx, _ = testclock.UseTime(ctx, testclock.TestTimeUTC)
		cfg := GetConfig(ctx)
		tq := taskqueue.Get(ctx)

		tq.Testable().CreateQueue(cfg.Name)

		Convey("empty", func() {
			So(fireTasks(ctx, nil), ShouldBeTrue)
			So(len(tq.Testable().GetScheduledTasks()[cfg.Name]), ShouldEqual, 0)
		})

		Convey("basic", func() {
			So(fireTasks(ctx, map[uint64]struct{}{2: {}, 7: {}}), ShouldBeTrue)
			q := tq.Testable().GetScheduledTasks()[cfg.Name]
			So(q["-62132730888_2"], ShouldResemble, &taskqueue.Task{
				Name:   "-62132730888_2",
				Method: "POST",
				Path:   cfg.ProcessURL(time.Unix(-62132730888, 0), 2),
				ETA:    testclock.TestTimeUTC.Add(6 * time.Second).Round(time.Second),
			})
			So(q["-62132730888_7"], ShouldResemble, &taskqueue.Task{
				Name:   "-62132730888_7",
				Method: "POST",
				Path:   cfg.ProcessURL(time.Unix(-62132730888, 0), 7),
				ETA:    testclock.TestTimeUTC.Add(6 * time.Second).Round(time.Second),
			})
		})
	})
}
