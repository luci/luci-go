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

package invocations

import (
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestByTimestamp(t *testing.T) {
	Convey(`ByTimestamp`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		realm := "testproject:bytimestamprealm"

		start := testclock.TestRecentTimeUTC
		middle := start.Add(time.Hour)
		end := middle.Add(time.Hour)

		// Insert some Invocations.
		testutil.MustApply(ctx,
			insertInvocation("first", map[string]interface{}{
				"CreateTime":  start,
				"HistoryTime": start,
				"Realm":       realm,
			}),
			insertInvocation("second", map[string]interface{}{
				"CreateTime":  middle,
				"HistoryTime": middle,
				"Realm":       realm,
			}),
			insertInvocation("secondWrongRealm", map[string]interface{}{
				"CreateTime":  middle,
				"HistoryTime": middle,
				"Realm":       "testproject:wrongrealm",
			}),
			insertInvocation("secondUnindexed", map[string]interface{}{
				"CreateTime": middle,
				// No HistoryTime.
				"Realm": realm,
			}),
			insertInvocation("third", map[string]interface{}{
				"CreateTime":  end,
				"HistoryTime": end,
				"Realm":       realm,
			}),
		)
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		beforePB, _ := ptypes.TimestampProto(start.Add(-1 * time.Hour))
		startPB, _ := ptypes.TimestampProto(start)
		middlePB, _ := ptypes.TimestampProto(middle)
		endPB, _ := ptypes.TimestampProto(end)
		afterPB, _ := ptypes.TimestampProto(end.Add(time.Hour))
		muchLaterPB, _ := ptypes.TimestampProto(end.Add(24 * time.Hour))

		// Some results, excluding the most recent.
		res, err := ByTimestamp(ctx, &pb.TimeRange{Earliest: startPB, Latest: endPB}, realm)
		So(err, ShouldBeNil)
		So(res, ShouldHaveLength, 2)
		So(string(res[0].ID), ShouldEndWith, "second")
		So(string(res[1].ID), ShouldEndWith, "first")

		// Some results, excluding the oldest.
		res, err = ByTimestamp(ctx, &pb.TimeRange{Earliest: middlePB, Latest: afterPB}, realm)
		So(err, ShouldBeNil)
		So(res, ShouldHaveLength, 2)
		So(string(res[0].ID), ShouldEndWith, "third")
		So(string(res[1].ID), ShouldEndWith, "second")

		// All results.
		res, err = ByTimestamp(ctx, &pb.TimeRange{Earliest: startPB, Latest: afterPB}, realm)
		So(err, ShouldBeNil)
		So(res, ShouldHaveLength, 3)
		So(string(res[0].ID), ShouldEndWith, "third")
		So(string(res[1].ID), ShouldEndWith, "second")
		So(string(res[2].ID), ShouldEndWith, "first")

		// Before first result.
		res, err = ByTimestamp(ctx, &pb.TimeRange{Earliest: beforePB, Latest: startPB}, realm)
		So(err, ShouldBeNil)
		So(res, ShouldHaveLength, 0)

		// After last result.
		res, err = ByTimestamp(ctx, &pb.TimeRange{Earliest: afterPB, Latest: muchLaterPB}, realm)
		So(err, ShouldBeNil)
		So(res, ShouldHaveLength, 0)
	})
}
