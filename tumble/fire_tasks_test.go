// Copyright 2015 The LUCI Authors.
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

package tumble

import (
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/memlogger"

	"github.com/luci/gae/service/info"
	tq "github.com/luci/gae/service/taskqueue"

	. "github.com/smartystreets/goconvey/convey"
)

func TestShardCalculation(t *testing.T) {
	t.Parallel()

	Convey("shard calculation", t, func() {
		tt := &Testing{}
		ctx := tt.Context()
		cfg := tt.GetConfig(ctx)
		cfg.NumShards = 11
		tt.UpdateSettings(ctx, cfg)

		l := logging.Get(ctx).(*memlogger.MemLogger)

		Convey("ExpandedShard->Shard covers the full int64 range", func() {
			tcs := []struct {
				es int64
				s  uint64
			}{
				{math.MinInt64, 0},
				{0, cfg.NumShards / 2},
				{math.MaxInt64, cfg.NumShards - 1},
			}

			for _, tc := range tcs {
				So((&realMutation{ExpandedShard: tc.es}).shard(cfg).shard, ShouldEqual, tc.s)
				low, high := expandedShardBounds(ctx, cfg, tc.s)
				So(tc.es, ShouldBeGreaterThanOrEqualTo, low)
				So(tc.es, ShouldBeLessThanOrEqualTo, high)
				So(l.Messages(), ShouldBeEmpty)
			}
		})

		Convey("expandedShardsPerShard returns crossed ranges on shard reduction", func() {
			low, high := expandedShardBounds(ctx, cfg, 256)
			So(low, ShouldBeGreaterThan, high)
			So(l, memlogger.ShouldHaveLog, logging.Warning, "Invalid shard: 256")
		})
	})
}

func TestFireTasks(t *testing.T) {
	t.Parallel()

	Convey("fireTasks works as expected", t, func() {
		tt := &Testing{}
		ctx := tt.Context()

		Convey("empty", func() {
			So(fireTasks(ctx, tt.GetConfig(ctx), nil, true), ShouldBeTrue)
			So(len(tq.GetTestable(ctx).GetScheduledTasks()[baseName]), ShouldEqual, 0)
		})

		Convey("basic", func() {
			So(fireTasks(ctx, tt.GetConfig(ctx), map[taskShard]struct{}{
				{2, minTS}: {},
				{7, minTS}: {},

				// since DelayedMutations is false, this timew will be reset
				{5, mkTimestamp(tt.GetConfig(ctx), testclock.TestTimeUTC.Add(time.Minute))}: {},
			}, true), ShouldBeTrue)
			So(tq.GetTestable(ctx).GetScheduledTasks()[baseName], ShouldResemble, map[string]*tq.Task{
				"-62132730888__2": {
					Name:   "-62132730888__2",
					Method: "POST",
					Path:   processURL(-62132730888, 2, "", true),
					ETA:    testclock.TestTimeUTC.Add(6 * time.Second).Round(time.Second),
				},
				"-62132730888__7": {
					Name:   "-62132730888__7",
					Method: "POST",
					Path:   processURL(-62132730888, 7, "", true),
					ETA:    testclock.TestTimeUTC.Add(6 * time.Second).Round(time.Second),
				},
				"-62132730888__5": {
					Name:   "-62132730888__5",
					Method: "POST",
					Path:   processURL(-62132730888, 5, "", true),
					ETA:    testclock.TestTimeUTC.Add(6 * time.Second).Round(time.Second),
				},
			})
		})

		Convey("namespaced", func() {
			ctx = info.MustNamespace(ctx, "foo.bar")
			So(fireTasks(ctx, tt.GetConfig(ctx), map[taskShard]struct{}{
				{2, minTS}: {},
			}, true), ShouldBeTrue)
			So(tq.GetTestable(ctx).GetScheduledTasks()[baseName], ShouldResemble, map[string]*tq.Task{
				"-62132730888_foo_bar_2": {
					Name:   "-62132730888_foo_bar_2",
					Method: "POST",
					Header: http.Header{
						"X-Appengine-Current-Namespace": []string{"foo.bar"},
					},
					Path: processURL(-62132730888, 2, "foo.bar", true),
					ETA:  testclock.TestTimeUTC.Add(6 * time.Second).Round(time.Second),
				},
			})
		})

		Convey("delayed", func() {
			cfg := tt.GetConfig(ctx)
			cfg.DelayedMutations = true
			tt.UpdateSettings(ctx, cfg)
			delayedTS := mkTimestamp(cfg, testclock.TestTimeUTC.Add(time.Minute*10))
			So(fireTasks(ctx, cfg, map[taskShard]struct{}{
				{1, delayedTS}: {},
			}, true), ShouldBeTrue)
			So(tq.GetTestable(ctx).GetScheduledTasks()[baseName], ShouldResemble, map[string]*tq.Task{
				"-62132730288__1": {
					Name:   "-62132730288__1",
					Method: "POST",
					Path:   processURL(-62132730288, 1, "", true),
					ETA:    testclock.TestTimeUTC.Add(time.Minute * 10).Add(6 * time.Second).Round(time.Second),
				},
			})
		})
	})
}
