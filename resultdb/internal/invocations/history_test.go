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
		start := testclock.TestRecentTimeUTC
		middle := start.Add(time.Hour)
		end := middle.Add(time.Hour)
		realm := "testproject/bytimestamprealm"

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
				"Realm":       "testproject/wrongrealm",
			}),
			insertInvocation("secondUnindexed", map[string]interface{}{
				"CreateTime": middle,
				"Realm":      realm,
			}),
			insertInvocation("third", map[string]interface{}{
				"CreateTime":  end,
				"HistoryTime": end,
				"Realm":       realm,
			}),
		)
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		beforePB, err := ptypes.TimestampProto(start.Add(-1 * time.Hour))
		So(err, ShouldBeNil)
		startPB, err := ptypes.TimestampProto(start)
		So(err, ShouldBeNil)
		endPB, err := ptypes.TimestampProto(end)
		So(err, ShouldBeNil)
		afterPB, err := ptypes.TimestampProto(end.Add(time.Hour))
		So(err, ShouldBeNil)
		muchAfterPB, err := ptypes.TimestampProto(end.Add(24 * time.Hour))
		So(err, ShouldBeNil)

		// start to end, returns second, then first
		res, err := ByTimestamp(ctx, &pb.TimeRange{
			Earliest: startPB,
			Latest:   endPB,
		}, 10, realm)
		So(err, ShouldBeNil)
		So(res, ShouldHaveLength, 2)
		So(res[0].Name, ShouldEndWith, "second")
		So(res[1].Name, ShouldEndWith, "first")

		// start ot end + 1, returns third, then second, then 1
		res, err = ByTimestamp(ctx, &pb.TimeRange{
			Earliest: startPB,
			Latest:   afterPB,
		}, 10, realm)
		So(err, ShouldBeNil)
		So(res, ShouldHaveLength, 3)
		So(res[0].Name, ShouldEndWith, "third")
		So(res[1].Name, ShouldEndWith, "second")
		So(res[2].Name, ShouldEndWith, "first")

		// start to end + 1, pageSize 2, returns third, then second.
		res, err = ByTimestamp(ctx, &pb.TimeRange{
			Earliest: startPB,
			Latest:   afterPB,
		}, 2, realm)
		So(err, ShouldBeNil)
		So(res, ShouldHaveLength, 2)
		So(res[0].Name, ShouldEndWith, "third")
		So(res[1].Name, ShouldEndWith, "second")

		res, err = ByTimestamp(ctx, &pb.TimeRange{
			Earliest: beforePB,
			Latest:   startPB,
		}, 10, realm)
		So(err, ShouldBeNil)
		So(res, ShouldHaveLength, 0)

		res, err = ByTimestamp(ctx, &pb.TimeRange{
			Earliest: afterPB,
			Latest:   muchAfterPB,
		}, 10, realm)
		So(err, ShouldBeNil)
		So(res, ShouldHaveLength, 0)

	})
}
