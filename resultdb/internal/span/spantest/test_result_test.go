// Copyright 2019 The LUCI Authors.
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

package spantest

import (
	"fmt"
	"testing"

	"cloud.google.com/go/spanner"
	durpb "github.com/golang/protobuf/ptypes/duration"

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestQueryTestResults(t *testing.T) {
	Convey(`QueryTestResults`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		now := clock.Now(ctx)

		testutil.MustApply(ctx, testutil.InsertInvocation("inv1", pb.Invocation_ACTIVE, "", now))
		q := span.TestResultQuery{
			Predicate:     &pb.TestResultPredicate{},
			PageSize:      100,
			InvocationIDs: []span.InvocationID{"inv1"},
		}

		read := func(q span.TestResultQuery) (trs []*pb.TestResult, token string, err error) {
			txn := span.Client(ctx).ReadOnlyTransaction()
			defer txn.Close()
			return span.QueryTestResults(ctx, txn, q)
		}

		mustRead := func(q span.TestResultQuery, expected []*pb.TestResult) string {
			trs, token, err := read(q)
			So(err, ShouldBeNil)
			So(trs, ShouldResembleProto, expected)
			return token
		}

		Convey(`Does not fetch test results of other invocations`, func() {
			expected := makeTestResults("inv1", "DoBaz",
				pb.TestStatus_PASS,
				pb.TestStatus_FAIL,
				pb.TestStatus_FAIL,
				pb.TestStatus_PASS,
				pb.TestStatus_FAIL,
			)
			testutil.MustApply(ctx,
				testutil.InsertInvocation("inv0", pb.Invocation_ACTIVE, "", now),
				testutil.InsertInvocation("inv2", pb.Invocation_ACTIVE, "", now),
			)
			testutil.MustApply(ctx, testutil.CombineMutations(
				insertTestResults(makeTestResults("inv0", "X", pb.TestStatus_PASS, pb.TestStatus_FAIL)),
				insertTestResults(expected),
				insertTestResults(makeTestResults("inv2", "Y", pb.TestStatus_PASS, pb.TestStatus_FAIL)),
			)...)

			mustRead(q, expected)
		})

		Convey(`Paging`, func() {
			trs := makeTestResults("inv1", "DoBaz",
				pb.TestStatus_PASS,
				pb.TestStatus_FAIL,
				pb.TestStatus_FAIL,
				pb.TestStatus_PASS,
				pb.TestStatus_FAIL,
			)
			testutil.MustApply(ctx, insertTestResults(trs)...)

			mustReadPage := func(pageToken string, pageSize int, expected []*pb.TestResult) string {
				q2 := q
				q2.CursorToken = pageToken
				q2.PageSize = pageSize
				return mustRead(q2, expected)
			}

			Convey(`All results`, func() {
				token := mustReadPage("", 10, trs)
				So(token, ShouldEqual, "")
			})

			Convey(`With pagination`, func() {
				token := mustReadPage("", 1, trs[:1])
				So(token, ShouldNotEqual, "")

				token = mustReadPage(token, 4, trs[1:])
				So(token, ShouldNotEqual, "")

				token = mustReadPage(token, 5, nil)
				So(token, ShouldEqual, "")
			})

			Convey(`Bad token`, func() {
				txn := span.Client(ctx).ReadOnlyTransaction()
				defer txn.Close()

				Convey(`From bad position`, func() {
					q.CursorToken = "CgVoZWxsbw=="
					_, _, err := span.QueryTestResults(ctx, txn, q)
					So(err, ShouldErrLike, "invalid page_token")
				})

				Convey(`From decoding`, func() {
					q.CursorToken = "%%%"
					_, _, err := span.QueryTestResults(ctx, txn, q)
					So(err, ShouldErrLike, "invalid page_token")
				})
			})
		})
	})
}

// insertTestResults returns spanner mutations to insert test results
func insertTestResults(trs []*pb.TestResult) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, len(trs))
	for i, tr := range trs {
		invID, testPath, resultID := span.MustParseTestResultName(tr.Name)
		mutMap := map[string]interface{}{
			"InvocationId":      invID,
			"TestPath":          testPath,
			"ResultId":          resultID,
			"ExtraVariantPairs": trs[i].ExtraVariantPairs,
			"CommitTimestamp":   spanner.CommitTimestamp,
			"Status":            tr.Status,
			"RunDurationUsec":   1e6*i + 234567,
		}
		if !trs[i].Expected {
			mutMap["IsUnexpected"] = true
		}

		ms[i] = span.InsertMap("TestResults", mutMap)
	}
	return ms
}

func makeTestResults(invID, testName string, statuses ...pb.TestStatus) []*pb.TestResult {
	trs := make([]*pb.TestResult, len(statuses))

	for i, status := range statuses {
		testPath := "gn:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest." + testName
		resultID := fmt.Sprintf("result_id_within_inv_%d", i)

		trs[i] = &pb.TestResult{
			Name:              pbutil.TestResultName(string(invID), testPath, resultID),
			TestPath:          testPath,
			ResultId:          resultID,
			ExtraVariantPairs: pbutil.Variant("k1", "v1", "k2", "v2"),
			Expected:          status == pb.TestStatus_PASS,
			Status:            status,
			Duration:          &durpb.Duration{Seconds: int64(i), Nanos: 234567000},
		}
	}
	return trs
}
