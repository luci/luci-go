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

package history

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/spanner"

	"github.com/golang/protobuf/ptypes"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestHistory(t *testing.T) {
	Convey(`TestHistory`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx = clock.Set(ctx, testclock.New(testclock.TestRecentTimeUTC.Add(time.Hour)))

		earliest, _ := ptypes.TimestampProto(testclock.TestRecentTimeUTC)
		latest, _ := ptypes.TimestampProto(testclock.TestRecentTimeUTC.Add(2 * time.Minute))
		afterLatest, _ := ptypes.TimestampProto(testclock.TestRecentTimeUTC.Add(3 * time.Minute))

		q := &Query{
			Request: &pb.GetTestResultHistoryRequest{
				Range: &pb.GetTestResultHistoryRequest_TimeRange{
					TimeRange: &pb.TimeRange{
						Earliest: earliest,
						Latest:   afterLatest,
					},
				},
				VariantPredicate: &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("dummy", "true"),
					},
				},
			},
		}

		// Insert 3 indexed invocations one minute apart,
		// with 3 unindexed child invocations each,
		// and with 3 test results in each child invocation.
		// Plus 10 invocations that are indexed but contain no results.
		// 22 invocations + 9 inclusions + 27 results = 58 mutations
		ms := make([]*spanner.Mutation, 0, 58)
		ms = insertResultHistoryData(ms, "some-invocation", 0, 3, 3)
		ms = insertResultHistoryData(ms, "some-other-invocation", time.Minute, 3, 3)
		ms = insertResultHistoryData(ms, "yet-another-invocation", 2*time.Minute, 3, 3)
		// Insert 20 indexed invocations with no results after the last
		// invocation that does contain results.
		for i := 0; i < 10; i++ {
			ms = insertOneResultsInv(ms, fmt.Sprintf("empty-%d", i), testclock.TestRecentTimeUTC.Add(2*time.Minute).Add(time.Duration(i)*time.Second), true, 0)
		}
		testutil.MustApply(ctx, ms...)

		var entries []*pb.GetTestResultHistoryResponse_Entry
		var nextPageToken string
		var err error

		Convey(`page token`, func() {
			q.Request.Realm = "testproject:testrealm"
			q.Request.PageSize = 5
			span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				entries, nextPageToken, err = q.Execute(ctx)
				return nil
			})
			So(err, ShouldBeNil)
			n := len(entries)
			So(n, ShouldBeLessThanOrEqualTo, q.Request.PageSize)
			parts, err := pagination.ParseToken(nextPageToken)
			So(err, ShouldBeNil)
			So(parts[0], ShouldEqual, "ts")
			So(parts[2], ShouldEqual, fmt.Sprintf("%d", n))
		})

		Convey(`paging`, func() {
			q.Request.Realm = "testproject:testrealm"
			q.Request.PageSize = 10
			span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				entries, nextPageToken, err = q.Execute(ctx)
				return nil
			})
			So(err, ShouldBeNil)
			So(entries, ShouldHaveLength, 10)

			// Get next page.
			q = &Query{Request: q.Request}
			q.Request.PageToken = nextPageToken
			span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				entries, nextPageToken, err = q.Execute(ctx)
				return nil
			})
			So(err, ShouldBeNil)
			So(entries, ShouldHaveLength, 10)

			// Get next page.
			q = &Query{Request: q.Request}
			q.Request.PageToken = nextPageToken
			span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				entries, nextPageToken, err = q.Execute(ctx)
				return nil
			})
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldEqual, "")
			So(entries, ShouldHaveLength, 7)
		})

		Convey(`out of time`, func() {
			q.Request.Realm = "testproject:testrealm"
			// Use system clock to avoid issues with spanner.
			ctx := clock.Set(ctx, clock.GetSystemClock())
			// Make a context that is just about to expire to force the api
			// to return partial results.
			expiringCtx, cancel := clock.WithTimeout(ctx, 4*time.Second)
			defer cancel()
			span.ReadWriteTransaction(expiringCtx, func(ctx context.Context) error {
				entries, nextPageToken, err = q.Execute(ctx)
				return nil
			})
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldNotEqual, "")
			So(entries, ShouldHaveLength, 9)
		})

		Convey(`all results`, func() {
			q.Request.Realm = "testproject:testrealm"
			span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				entries, nextPageToken, err = q.Execute(ctx)
				return nil
			})
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldEqual, "")
			So(entries, ShouldHaveLength, 27)
			for i := 0; i < 9; i++ {
				So(entries[i].InvocationTimestamp, ShouldResembleProto, latest)
			}
			for i := 19; i < 27; i++ {
				So(entries[i].InvocationTimestamp, ShouldResembleProto, earliest)
			}
		})
	})

}

func insertResultHistoryData(ms []*spanner.Mutation, id string, offset time.Duration, subInvs, results int) []*spanner.Mutation {
	// Insert indexed invocation with subinvocations and results.
	ms = insertOneResultsInv(ms, id, testclock.TestRecentTimeUTC.Add(offset), true, 0)
	for i := 0; i < subInvs; i++ {
		// Insert result-containing invocations contained by the above.
		idSub := fmt.Sprintf("%s-%d", id, i+1)
		ms = insertOneResultsInv(ms, idSub, testclock.TestRecentTimeUTC, false, results)
		ms = append(ms, insert.Inclusion(invocations.ID(id), invocations.ID(idSub)))
	}
	return ms
}

// insertOneResultsInv inserts one invocation,
// optionally indexes it by timestamp,
// and, also optionally, adds some test results contained by it.
func insertOneResultsInv(ms []*spanner.Mutation, id string, ts time.Time, indexed bool, results int) []*spanner.Mutation {
	extraArgs := map[string]interface{}{
		"CreateTime": ts,
		"Realm":      "testproject:testrealm",
	}
	if indexed {
		extraArgs["HistoryTime"] = ts
	}
	invID := invocations.ID(id)
	ms = append(ms, insert.Invocation(invID, pb.Invocation_ACTIVE, extraArgs))
	for i := 0; i < results; i++ {
		ms = append(ms,
			spanutil.InsertMap("TestResults", map[string]interface{}{
				"InvocationId":    invID,
				"TestId":          fmt.Sprintf("ninja://chrome/test:foo_tests/BarTest.DoBaz-%s", id),
				"ResultId":        fmt.Sprintf("%d", i),
				"Variant":         pbutil.Variant("result_index", fmt.Sprintf("%d", i), "dummy", "true"),
				"VariantHash":     fmt.Sprintf("deadbeef%d", i),
				"CommitTimestamp": spanner.CommitTimestamp,
				"Status":          pb.TestStatus_PASS,
				"RunDurationUsec": 1534567,
			}))
	}
	return ms
}
