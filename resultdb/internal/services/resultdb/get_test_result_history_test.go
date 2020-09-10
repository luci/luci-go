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

package resultdb

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateGetTestResultHistoryRequest(t *testing.T) {
	t.Parallel()

	Convey(`ValidateGetTestResultHistoryRequest`, t, func() {
		recently, _ := ptypes.TimestampProto(testclock.TestRecentTimeUTC)
		earlier, _ := ptypes.TimestampProto(testclock.TestRecentTimeUTC.Add(-24 * time.Hour))

		Convey(`valid`, func() {
			req := &pb.GetTestResultHistoryRequest{
				Realm: "testproject:testrealm",
				Range: &pb.GetTestResultHistoryRequest_TimeRange{
					TimeRange: &pb.TimeRange{
						Earliest: earlier,
						Latest:   recently,
					},
				},
				PageSize:     10,
				TestIdRegexp: "ninja://:blink_web_tests/fast/.*",
				VariantPredicate: &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: &pb.Variant{
							Def: map[string]string{"os": "Mac-10.15"},
						},
					},
				},
			}
			So(validateGetTestResultHistoryRequest(req), ShouldBeNil)
		})

		Convey(`missing realm`, func() {
			req := &pb.GetTestResultHistoryRequest{
				Range: &pb.GetTestResultHistoryRequest_TimeRange{
					TimeRange: &pb.TimeRange{
						Earliest: earlier,
						Latest:   recently,
					},
				},
				PageSize:     10,
				TestIdRegexp: "ninja://:blink_web_tests/fast/.*",
				VariantPredicate: &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: &pb.Variant{
							Def: map[string]string{"os": "Mac-10.15"},
						},
					},
				},
			}
			So(validateGetTestResultHistoryRequest(req), ShouldErrLike, "realm is required")
		})

		Convey(`bad pageSize`, func() {
			req := &pb.GetTestResultHistoryRequest{
				Realm: "testproject:testrealm",
				Range: &pb.GetTestResultHistoryRequest_TimeRange{
					TimeRange: &pb.TimeRange{
						Earliest: earlier,
						Latest:   recently,
					},
				},
				PageSize:     -10,
				TestIdRegexp: "ninja://:blink_web_tests/fast/.*",
				VariantPredicate: &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: &pb.Variant{
							Def: map[string]string{"os": "Mac-10.15"},
						},
					},
				},
			}
			So(validateGetTestResultHistoryRequest(req), ShouldErrLike, "page_size, if specified, must be a positive integer")
		})

		Convey(`bad variant`, func() {
			req := &pb.GetTestResultHistoryRequest{
				Realm: "testproject:testrealm",
				Range: &pb.GetTestResultHistoryRequest_TimeRange{
					TimeRange: &pb.TimeRange{
						Earliest: earlier,
						Latest:   recently,
					},
				},
				PageSize:     10,
				TestIdRegexp: "ninja://:blink_web_tests/fast/.*",
				VariantPredicate: &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: &pb.Variant{
							Def: map[string]string{"$os": "Mac-10.15"},
						},
					},
				},
			}
			So(validateGetTestResultHistoryRequest(req), ShouldErrLike, "variant_predicate")
		})
	})
}

func TestGetTestResultHistory(t *testing.T) {
	Convey(`TestGetTestResultHistoryRequest`, t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: permGetTestResult},
			},
		})
		srv := newTestResultDBService()

		// Insert 3 indexed invocations one minute apart,
		// with 3 unindexed child invocations each,
		// and with 3 test results in each child invocation.
		insertResultHistoryData(ctx, "some-invocation", time.Duration(0), 3, 3)
		insertResultHistoryData(ctx, "some-other-invocation", time.Duration(1)*time.Minute, 3, 3)
		insertResultHistoryData(ctx, "yet-another-invocation", time.Duration(2)*time.Minute, 3, 3)

		earliest, _ := ptypes.TimestampProto(testclock.TestRecentTimeUTC)
		latest, _ := ptypes.TimestampProto(testclock.TestRecentTimeUTC.Add(time.Duration(2) * time.Minute))

		req := &pb.GetTestResultHistoryRequest{
			Range: &pb.GetTestResultHistoryRequest_TimeRange{
				TimeRange: &pb.TimeRange{
					Earliest: earliest,
					Latest:   latest,
				},
			},
			VariantPredicate: &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_Contains{
					Contains: &pb.Variant{
						Def: map[string]string{"dummy": "true"},
					},
				},
			},
		}

		Convey(`no realm`, func() {
			res, err := srv.GetTestResultHistory(ctx, req)
			So(err, ShouldHaveAppStatus, codes.InvalidArgument)
			So(res, ShouldBeNil)
		})

		Convey(`unauthorized`, func() {
			req.Realm = "testproject:secretrealm"
			res, err := srv.GetTestResultHistory(ctx, req)
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
			So(res, ShouldBeNil)
		})

		Convey(`truncated`, func() {
			req.Realm = "testproject:testrealm"
			req.PageSize = 5
			res, err := srv.GetTestResultHistory(ctx, req)
			So(err, ShouldBeNil)
			So(res.Entries, ShouldHaveLength, 5)
		})

		Convey(`all results`, func() {
			req.Realm = "testproject:testrealm"
			res, err := srv.GetTestResultHistory(ctx, req)
			So(err, ShouldBeNil)
			So(res.Entries, ShouldHaveLength, 18)

			So(res.Entries[0].InvocationTimestamp.AsTime().Before(latest.AsTime()), ShouldBeTrue)
			So(res.Entries[0].Result.ResultId, ShouldEndWith, "some-other-invocation-1")
			So(res.Entries[3].Result.ResultId, ShouldEndWith, "some-other-invocation-2")
			So(res.Entries[6].Result.ResultId, ShouldEndWith, "some-other-invocation-3")

			So(res.Entries[9].InvocationTimestamp, ShouldResembleProto, earliest)
			So(res.Entries[9].Result.ResultId, ShouldEndWith, "some-invocation-1")
			So(res.Entries[12].Result.ResultId, ShouldEndWith, "some-invocation-2")
			So(res.Entries[15].Result.ResultId, ShouldEndWith, "some-invocation-3")
		})
	})
}

func insertResultHistoryData(ctx context.Context, id string, offset time.Duration, subInvs, results int) {
	// Insert indexed invocation with subinvocations and results.
	insertOneResultsInv(ctx, id, testclock.TestRecentTimeUTC.Add(offset), true, 0)
	for i := 0; i < subInvs; i++ {
		// Insert result-containing invocations contained by the above.
		idSub := fmt.Sprintf("%s-%d", id, i+1)
		insertOneResultsInv(ctx, idSub, testclock.TestRecentTimeUTC, false, results)
		testutil.MustApply(ctx, insert.Inclusion(invocations.ID(id), invocations.ID(idSub)))
	}
}

// insertOneResultsInv inserts one invocation,
// optionally indexes it by timestamp,
// and, also optionally, adds some test results contained by it.
func insertOneResultsInv(ctx context.Context, id string, ts time.Time, indexed bool, results int) {
	extraArgs := map[string]interface{}{
		"CreateTime": ts,
		"Realm":      "testproject:testrealm",
	}
	if indexed {
		extraArgs["HistoryTime"] = ts
	}
	invID := invocations.ID(id)
	testutil.MustApply(ctx, insert.Invocation(invID, pb.Invocation_ACTIVE, extraArgs))
	for i := 0; i < results; i++ {
		testutil.MustApply(ctx, spanutil.InsertMap("TestResults", map[string]interface{}{
			"InvocationId":    invID,
			"TestId":          "ninja://chrome/test:foo_tests/BarTest.DoBaz",
			"ResultId":        fmt.Sprintf("result_%d_within_inv_%s", i, id),
			"Variant":         pbutil.Variant("result_index", fmt.Sprintf("%d", i), "dummy", "true"),
			"VariantHash":     "deadbeef",
			"CommitTimestamp": spanner.CommitTimestamp,
			"Status":          pb.TestStatus_PASS,
			"RunDurationUsec": 1534567,
		}))
	}
}
