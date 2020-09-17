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
		justBeforePB, _ := ptypes.TimestampProto(start.Add(-1 * time.Second))
		startPB, _ := ptypes.TimestampProto(start)
		middlePB, _ := ptypes.TimestampProto(middle)
		afterPB, _ := ptypes.TimestampProto(end.Add(time.Hour))
		muchLaterPB, _ := ptypes.TimestampProto(end.Add(24 * time.Hour))

		Convey(`all but the most recent`, func() {
			res, err := ByTimestamp(ctx, realm, &pb.TimeRange{Earliest: startPB, Latest: middlePB})
			So(err, ShouldBeNil)
			So(res, ShouldHaveLength, 2)
			So(string(res[0].ID), ShouldEndWith, "second")
			So(string(res[1].ID), ShouldEndWith, "first")
		})

		Convey(`all but the oldest`, func() {
			res, err := ByTimestamp(ctx, realm, &pb.TimeRange{Earliest: middlePB, Latest: afterPB})
			So(err, ShouldBeNil)
			So(res, ShouldHaveLength, 2)
			So(string(res[0].ID), ShouldEndWith, "third")
			So(string(res[1].ID), ShouldEndWith, "second")
		})

		Convey(`all results`, func() {
			res, err := ByTimestamp(ctx, realm, &pb.TimeRange{Earliest: startPB, Latest: afterPB})
			So(err, ShouldBeNil)
			So(res, ShouldHaveLength, 3)
			So(string(res[0].ID), ShouldEndWith, "third")
			So(string(res[1].ID), ShouldEndWith, "second")
			So(string(res[2].ID), ShouldEndWith, "first")
		})

		Convey(`before first result`, func() {
			res, err := ByTimestamp(ctx, realm, &pb.TimeRange{Earliest: beforePB, Latest: justBeforePB})
			So(err, ShouldBeNil)
			So(res, ShouldHaveLength, 0)
		})

		Convey(`after last result`, func() {
			res, err := ByTimestamp(ctx, realm, &pb.TimeRange{Earliest: afterPB, Latest: muchLaterPB})
			So(err, ShouldBeNil)
			So(res, ShouldHaveLength, 0)
		})
	})
}
