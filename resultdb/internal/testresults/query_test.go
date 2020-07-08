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

package testresults

import (
	"sort"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestQueryTestResults(t *testing.T) {
	Convey(`QueryTestResults`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		testutil.MustApply(ctx, insert.Invocation("inv1", pb.Invocation_ACTIVE, nil))

		q := &Query{
			Predicate:     &pb.TestResultPredicate{},
			PageSize:      100,
			InvocationIDs: invocations.NewIDSet("inv1"),
			Mask:          AllFields,
		}

		fetch := func(q *Query) (trs []*pb.TestResult, token string, err error) {
			txn := span.Client(ctx).ReadOnlyTransaction()
			defer txn.Close()
			return q.Fetch(ctx, txn)
		}

		mustFetch := func(q *Query) (trs []*pb.TestResult, token string) {
			trs, token, err := fetch(q)
			So(err, ShouldBeNil)
			return
		}

		mustFetchNames := func(q *Query) []string {
			trs, _, err := fetch(q)
			So(err, ShouldBeNil)
			names := make([]string, len(trs))
			for i, a := range trs {
				names[i] = a.Name
			}
			sort.Strings(names)
			return names
		}

		Convey(`Does not fetch test results of other invocations`, func() {
			expected := insert.MakeTestResults("inv1", "DoBaz", nil,
				pb.TestStatus_PASS,
				pb.TestStatus_FAIL,
				pb.TestStatus_FAIL,
				pb.TestStatus_PASS,
				pb.TestStatus_FAIL,
			)
			testutil.MustApply(ctx,
				insert.Invocation("inv0", pb.Invocation_ACTIVE, nil),
				insert.Invocation("inv2", pb.Invocation_ACTIVE, nil),
			)
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.TestResults("inv0", "X", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestResultMessages(expected),
				insert.TestResults("inv2", "Y", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
			)...)

			actual, _ := mustFetch(q)
			So(actual, ShouldResembleProto, expected)
		})

		Convey(`Expectancy filter`, func() {
			testutil.MustApply(ctx, insert.Invocation("inv0", pb.Invocation_ACTIVE, nil))
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.TestResults("inv0", "T1", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestResults("inv0", "T2", nil, pb.TestStatus_PASS),
				insert.TestResults("inv1", "T1", nil, pb.TestStatus_PASS),
				insert.TestResults("inv1", "T2", nil, pb.TestStatus_FAIL),
				insert.TestResults("inv1", "T3", nil, pb.TestStatus_PASS),
				insert.TestResults("inv1", "T4", nil, pb.TestStatus_FAIL),
			)...)

			q.InvocationIDs = invocations.NewIDSet("inv0", "inv1")
			q.Predicate.Expectancy = pb.TestResultPredicate_VARIANTS_WITH_UNEXPECTED_RESULTS
			actual, _ := mustFetch(q)
			pbutil.NormalizeTestResultSlice(actual)

			So(mustFetchNames(q), ShouldResemble, []string{
				"invocations/inv0/tests/T1/results/0",
				"invocations/inv0/tests/T1/results/1",
				"invocations/inv0/tests/T2/results/0",
				"invocations/inv1/tests/T1/results/0",
				"invocations/inv1/tests/T2/results/0",
				"invocations/inv1/tests/T4/results/0",
			})
		})

		Convey(`Test id filter`, func() {
			testutil.MustApply(ctx, insert.Invocation("inv0", pb.Invocation_ACTIVE, nil))
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.TestResults("inv0", "1-1", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestResults("inv0", "1-2", nil, pb.TestStatus_PASS),
				insert.TestResults("inv1", "1-1", nil, pb.TestStatus_PASS),
				insert.TestResults("inv1", "2-1", nil, pb.TestStatus_PASS),
				insert.TestResults("inv1", "2", nil, pb.TestStatus_FAIL),
			)...)

			q.InvocationIDs = invocations.NewIDSet("inv0", "inv1")
			q.Predicate.TestIdRegexp = "1-.+"

			So(mustFetchNames(q), ShouldResemble, []string{
				"invocations/inv0/tests/1-1/results/0",
				"invocations/inv0/tests/1-1/results/1",
				"invocations/inv0/tests/1-2/results/0",
				"invocations/inv1/tests/1-1/results/0",
			})
		})

		Convey(`Variant equals`, func() {
			testutil.MustApply(ctx, insert.Invocation("inv0", pb.Invocation_ACTIVE, nil))

			v1 := pbutil.Variant("k", "1")
			v2 := pbutil.Variant("k", "2")
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.TestResults("inv0", "1-1", v1, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestResults("inv0", "1-2", v2, pb.TestStatus_PASS),
				insert.TestResults("inv1", "1-1", v1, pb.TestStatus_PASS),
				insert.TestResults("inv1", "2-1", v2, pb.TestStatus_PASS),
			)...)

			q.InvocationIDs = invocations.NewIDSet("inv0", "inv1")
			q.Predicate.Variant = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_Equals{Equals: v1},
			}

			So(mustFetchNames(q), ShouldResemble, []string{
				"invocations/inv0/tests/1-1/results/0",
				"invocations/inv0/tests/1-1/results/1",
				"invocations/inv1/tests/1-1/results/0",
			})
		})

		Convey(`Variant contains`, func() {
			testutil.MustApply(ctx, insert.Invocation("inv0", pb.Invocation_ACTIVE, nil))

			v1 := pbutil.Variant("k", "1")
			v11 := pbutil.Variant("k", "1", "k2", "1")
			v2 := pbutil.Variant("k", "2")
			testutil.MustApply(ctx, testutil.CombineMutations(
				insert.TestResults("inv0", "1-1", v1, pb.TestStatus_PASS, pb.TestStatus_FAIL),
				insert.TestResults("inv0", "1-2", v11, pb.TestStatus_PASS),
				insert.TestResults("inv1", "1-1", v1, pb.TestStatus_PASS),
				insert.TestResults("inv1", "2-1", v2, pb.TestStatus_PASS),
			)...)

			q.InvocationIDs = invocations.NewIDSet("inv0", "inv1")

			Convey(`Empty`, func() {
				q.Predicate.Variant = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{Contains: pbutil.Variant()},
				}

				So(mustFetchNames(q), ShouldResemble, []string{
					"invocations/inv0/tests/1-1/results/0",
					"invocations/inv0/tests/1-1/results/1",
					"invocations/inv0/tests/1-2/results/0",
					"invocations/inv1/tests/1-1/results/0",
					"invocations/inv1/tests/2-1/results/0",
				})
			})

			Convey(`Non-empty`, func() {
				q.Predicate.Variant = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{Contains: v1},
				}

				So(mustFetchNames(q), ShouldResemble, []string{
					"invocations/inv0/tests/1-1/results/0",
					"invocations/inv0/tests/1-1/results/1",
					"invocations/inv0/tests/1-2/results/0",
					"invocations/inv1/tests/1-1/results/0",
				})
			})
		})

		Convey(`Paging`, func() {
			trs := insert.MakeTestResults("inv1", "DoBaz", nil,
				pb.TestStatus_PASS,
				pb.TestStatus_FAIL,
				pb.TestStatus_FAIL,
				pb.TestStatus_PASS,
				pb.TestStatus_FAIL,
			)
			testutil.MustApply(ctx, insert.TestResultMessages(trs)...)

			mustReadPage := func(pageToken string, pageSize int, expected []*pb.TestResult) string {
				q2 := q
				q2.PageToken = pageToken
				q2.PageSize = pageSize
				actual, token := mustFetch(q2)
				So(actual, ShouldResembleProto, expected)
				return token
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
					q.PageToken = "CgVoZWxsbw=="
					_, _, err := q.Fetch(ctx, txn)
					So(err, ShouldHaveAppStatus, codes.InvalidArgument, "invalid page_token")
				})

				Convey(`From decoding`, func() {
					q.PageToken = "%%%"
					_, _, err := q.Fetch(ctx, txn)
					So(err, ShouldHaveAppStatus, codes.InvalidArgument, "invalid page_token")
				})
			})
		})
	})
}
