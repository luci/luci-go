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
	"github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

// historyAccumulator has a method that can be passed as a callback to ByTimestamp.
type historyAccumulator struct {
	invs []ID
	b    spanutil.Buffer
}

func (ia *historyAccumulator) accumulate(inv ID, ts *timestamp.Timestamp) error {
	// We only need the invocationID in these tests.
	_ = ts
	ia.invs = append(ia.invs, inv)
	return nil
}

func newHistoryAccumulator(capacity int) historyAccumulator {
	return historyAccumulator{
		invs: make([]ID, 0, capacity),
	}
}

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
			ac := newHistoryAccumulator(2)
			err := ByTimestamp(ctx, realm, &pb.TimeRange{Earliest: startPB, Latest: middlePB}, ac.accumulate)
			So(err, ShouldBeNil)
			So(ac.invs, ShouldHaveLength, 2)
			So(string(ac.invs[0]), ShouldEndWith, "second")
			So(string(ac.invs[1]), ShouldEndWith, "first")
		})

		Convey(`all but the oldest`, func() {
			ac := newHistoryAccumulator(2)
			err := ByTimestamp(ctx, realm, &pb.TimeRange{Earliest: middlePB, Latest: afterPB}, ac.accumulate)
			So(err, ShouldBeNil)
			So(ac.invs, ShouldHaveLength, 2)
			So(string(ac.invs[0]), ShouldEndWith, "third")
			So(string(ac.invs[1]), ShouldEndWith, "second")
		})

		Convey(`all results`, func() {
			ac := newHistoryAccumulator(2)
			err := ByTimestamp(ctx, realm, &pb.TimeRange{Earliest: startPB, Latest: afterPB}, ac.accumulate)
			So(err, ShouldBeNil)
			So(ac.invs, ShouldHaveLength, 3)
			So(string(ac.invs[0]), ShouldEndWith, "third")
			So(string(ac.invs[1]), ShouldEndWith, "second")
			So(string(ac.invs[2]), ShouldEndWith, "first")
		})

		Convey(`before first result`, func() {
			ac := newHistoryAccumulator(1)
			err := ByTimestamp(ctx, realm, &pb.TimeRange{Earliest: beforePB, Latest: justBeforePB}, ac.accumulate)
			So(err, ShouldBeNil)
			So(ac.invs, ShouldHaveLength, 0)
		})

		Convey(`after last result`, func() {
			ac := newHistoryAccumulator(1)
			err := ByTimestamp(ctx, realm, &pb.TimeRange{Earliest: afterPB, Latest: muchLaterPB}, ac.accumulate)
			So(err, ShouldBeNil)
			So(ac.invs, ShouldHaveLength, 0)
		})
	})
}
