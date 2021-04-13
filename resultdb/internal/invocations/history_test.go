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
				"CreateTime":         start,
				"HistoryTime":        start,
				"Realm":              realm,
				"CommonTestIDPrefix": "ninja://browser_tests/",
			}),
			insertInvocation("second", map[string]interface{}{
				"CreateTime":         middle,
				"HistoryTime":        middle,
				"Realm":              realm,
				"CommonTestIDPrefix": "",
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
				"CreateTime":         end,
				"HistoryTime":        end,
				"Realm":              realm,
				"CommonTestIDPrefix": "ninja://",
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

		q := &HistoryQuery{
			Realm: realm,
		}

		Convey(`all but the most recent`, func() {
			ac := newHistoryAccumulator(2)
			q.TimeRange = &pb.TimeRange{Earliest: startPB, Latest: middlePB}
			err := q.ByTimestamp(ctx, ac.accumulate)
			So(err, ShouldBeNil)
			So(ac.invs, ShouldHaveLength, 2)
			So(string(ac.invs[0]), ShouldEndWith, "second")
			So(string(ac.invs[1]), ShouldEndWith, "first")
		})

		Convey(`all but the oldest`, func() {
			ac := newHistoryAccumulator(2)
			q.TimeRange = &pb.TimeRange{Earliest: middlePB, Latest: afterPB}
			err := q.ByTimestamp(ctx, ac.accumulate)
			So(err, ShouldBeNil)
			So(ac.invs, ShouldHaveLength, 2)
			So(string(ac.invs[0]), ShouldEndWith, "third")
			So(string(ac.invs[1]), ShouldEndWith, "second")
		})

		Convey(`all results`, func() {
			ac := newHistoryAccumulator(2)
			q.TimeRange = &pb.TimeRange{Earliest: startPB, Latest: afterPB}
			err := q.ByTimestamp(ctx, ac.accumulate)
			So(err, ShouldBeNil)
			So(ac.invs, ShouldHaveLength, 3)
			So(string(ac.invs[0]), ShouldEndWith, "third")
			So(string(ac.invs[1]), ShouldEndWith, "second")
			So(string(ac.invs[2]), ShouldEndWith, "first")
		})

		Convey(`before first result`, func() {
			ac := newHistoryAccumulator(1)
			q.TimeRange = &pb.TimeRange{Earliest: beforePB, Latest: justBeforePB}
			err := q.ByTimestamp(ctx, ac.accumulate)
			So(err, ShouldBeNil)
			So(ac.invs, ShouldHaveLength, 0)
		})

		Convey(`after last result`, func() {
			ac := newHistoryAccumulator(1)
			q.TimeRange = &pb.TimeRange{Earliest: afterPB, Latest: muchLaterPB}
			err := q.ByTimestamp(ctx, ac.accumulate)
			So(err, ShouldBeNil)
			So(ac.invs, ShouldHaveLength, 0)
		})

		// Test cases for testIdRegexp.
		Convey(`testIdRegexp with non literal prefix`, func() {
			ac := newHistoryAccumulator(2)
			q.TimeRange = &pb.TimeRange{Earliest: startPB, Latest: afterPB}
			q.Predicate = &pb.TestResultPredicate{TestIdRegexp: ".*random_prefix"}
			err := q.ByTimestamp(ctx, ac.accumulate)
			So(err, ShouldBeNil)
			So(ac.invs, ShouldHaveLength, 3)
			So(string(ac.invs[0]), ShouldEndWith, "third")
			So(string(ac.invs[1]), ShouldEndWith, "second")
			So(string(ac.invs[2]), ShouldEndWith, "first")
		})

		Convey(`unmatched testIdRegexp`, func() {
			ac := newHistoryAccumulator(2)
			q.TimeRange = &pb.TimeRange{Earliest: startPB, Latest: afterPB}
			q.Predicate = &pb.TestResultPredicate{TestIdRegexp: "random_prefix"}
			err := q.ByTimestamp(ctx, ac.accumulate)
			So(err, ShouldBeNil)
			So(ac.invs, ShouldHaveLength, 1)
			So(string(ac.invs[0]), ShouldEndWith, "second")
		})

		Convey(`matched testIdRegexp`, func() {
			ac := newHistoryAccumulator(2)
			q.TimeRange = &pb.TimeRange{Earliest: startPB, Latest: afterPB}
			q.Predicate = &pb.TestResultPredicate{TestIdRegexp: "ninja://browser_.*abc"}
			err := q.ByTimestamp(ctx, ac.accumulate)
			So(err, ShouldBeNil)
			So(ac.invs, ShouldHaveLength, 3)
			So(string(ac.invs[0]), ShouldEndWith, "third")
			So(string(ac.invs[1]), ShouldEndWith, "second")
			So(string(ac.invs[2]), ShouldEndWith, "first")
		})

	})
}
