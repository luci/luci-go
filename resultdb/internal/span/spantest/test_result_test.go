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
	"sort"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	. "go.chromium.org/luci/resultdb/internal/testutil"
)

func TestMustParseTestResultName(t *testing.T) {
	t.Parallel()

	Convey("MustParseTestResultName", t, func() {
		Convey("Parse", func() {
			invID, testID, resultID := span.MustParseTestResultName(
				"invocations/a/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result5")
			So(invID, ShouldEqual, "a")
			So(testID, ShouldEqual, "ninja://chrome/test:foo_tests/BarTest.DoBaz")
			So(resultID, ShouldEqual, "result5")
		})

		Convey("Invalid", func() {
			invalidNames := []string{
				"invocations/a/tests/b",
				"invocations/a/tests/b/exonerations/c",
			}
			for _, name := range invalidNames {
				name := name
				So(func() { span.MustParseTestResultName(name) }, ShouldPanic)
			}
		})
	})
}

func TestQueryTestResults(t *testing.T) {
	Convey(`QueryTestResults`, t, func() {
		ctx := SpannerTestContext(t)

		MustApply(ctx, InsertInvocation("inv1", pb.Invocation_ACTIVE, nil))
		q := span.TestResultQuery{
			Predicate:     &pb.TestResultPredicate{},
			PageSize:      100,
			InvocationIDs: span.NewInvocationIDSet("inv1"),
		}

		makeTestResults := MakeTestResults
		insertTestResults := InsertTestResults

		read := func(q span.TestResultQuery) (trs []*pb.TestResult, token string, err error) {
			txn := span.Client(ctx).ReadOnlyTransaction()
			defer txn.Close()
			return span.QueryTestResults(ctx, txn, q)
		}

		mustRead := func(q span.TestResultQuery) (trs []*pb.TestResult, token string) {
			trs, token, err := read(q)
			So(err, ShouldBeNil)
			return
		}

		mustReadNames := func(q span.TestResultQuery) []string {
			trs, _, err := read(q)
			So(err, ShouldBeNil)
			names := make([]string, len(trs))
			for i, a := range trs {
				names[i] = a.Name
			}
			sort.Strings(names)
			return names
		}

		Convey(`Does not fetch test results of other invocations`, func() {
			expected := makeTestResults("inv1", "DoBaz", nil,
				pb.TestStatus_PASS,
				pb.TestStatus_FAIL,
				pb.TestStatus_FAIL,
				pb.TestStatus_PASS,
				pb.TestStatus_FAIL,
			)
			MustApply(ctx,
				InsertInvocation("inv0", pb.Invocation_ACTIVE, nil),
				InsertInvocation("inv2", pb.Invocation_ACTIVE, nil),
			)
			MustApply(ctx, CombineMutations(
				insertTestResults(makeTestResults("inv0", "X", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL)),
				insertTestResults(expected),
				insertTestResults(makeTestResults("inv2", "Y", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL)),
			)...)

			actual, _ := mustRead(q)
			So(actual, ShouldResembleProto, expected)
		})

		Convey(`Expectancy filter`, func() {
			MustApply(ctx, InsertInvocation("inv0", pb.Invocation_ACTIVE, nil))
			MustApply(ctx, CombineMutations(
				insertTestResults(makeTestResults("inv0", "T1", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL)),
				insertTestResults(makeTestResults("inv0", "T2", nil, pb.TestStatus_PASS)),
				insertTestResults(makeTestResults("inv1", "T1", nil, pb.TestStatus_PASS)),
				insertTestResults(makeTestResults("inv1", "T2", nil, pb.TestStatus_FAIL)),
				insertTestResults(makeTestResults("inv1", "T3", nil, pb.TestStatus_PASS)),
				insertTestResults(makeTestResults("inv1", "T4", nil, pb.TestStatus_FAIL)),
			)...)

			q.InvocationIDs = span.NewInvocationIDSet("inv0", "inv1")
			q.Predicate.Expectancy = pb.TestResultPredicate_VARIANTS_WITH_UNEXPECTED_RESULTS
			actual, _ := mustRead(q)
			pbutil.NormalizeTestResultSlice(actual)

			So(mustReadNames(q), ShouldResemble, []string{
				"invocations/inv0/tests/T1/results/0",
				"invocations/inv0/tests/T1/results/1",
				"invocations/inv0/tests/T2/results/0",
				"invocations/inv1/tests/T1/results/0",
				"invocations/inv1/tests/T2/results/0",
				"invocations/inv1/tests/T4/results/0",
			})
		})

		Convey(`Test id filter`, func() {
			MustApply(ctx, InsertInvocation("inv0", pb.Invocation_ACTIVE, nil))
			MustApply(ctx, CombineMutations(
				insertTestResults(makeTestResults("inv0", "1-1", nil, pb.TestStatus_PASS, pb.TestStatus_FAIL)),
				insertTestResults(makeTestResults("inv0", "1-2", nil, pb.TestStatus_PASS)),
				insertTestResults(makeTestResults("inv1", "1-1", nil, pb.TestStatus_PASS)),
				insertTestResults(makeTestResults("inv1", "2-1", nil, pb.TestStatus_PASS)),
				insertTestResults(makeTestResults("inv1", "2", nil, pb.TestStatus_FAIL)),
			)...)

			q.InvocationIDs = span.NewInvocationIDSet("inv0", "inv1")
			q.Predicate.TestIdRegexp = "1-.+"

			So(mustReadNames(q), ShouldResemble, []string{
				"invocations/inv0/tests/1-1/results/0",
				"invocations/inv0/tests/1-1/results/1",
				"invocations/inv0/tests/1-2/results/0",
				"invocations/inv1/tests/1-1/results/0",
			})
		})

		Convey(`Variant equals`, func() {
			MustApply(ctx, InsertInvocation("inv0", pb.Invocation_ACTIVE, nil))

			v1 := pbutil.Variant("k", "1")
			v2 := pbutil.Variant("k", "2")
			MustApply(ctx, CombineMutations(
				insertTestResults(makeTestResults("inv0", "1-1", v1, pb.TestStatus_PASS, pb.TestStatus_FAIL)),
				insertTestResults(makeTestResults("inv0", "1-2", v2, pb.TestStatus_PASS)),
				insertTestResults(makeTestResults("inv1", "1-1", v1, pb.TestStatus_PASS)),
				insertTestResults(makeTestResults("inv1", "2-1", v2, pb.TestStatus_PASS)),
			)...)

			q.InvocationIDs = span.NewInvocationIDSet("inv0", "inv1")
			q.Predicate.Variant = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_Equals{Equals: v1},
			}

			So(mustReadNames(q), ShouldResemble, []string{
				"invocations/inv0/tests/1-1/results/0",
				"invocations/inv0/tests/1-1/results/1",
				"invocations/inv1/tests/1-1/results/0",
			})
		})

		Convey(`Variant contains`, func() {
			MustApply(ctx, InsertInvocation("inv0", pb.Invocation_ACTIVE, nil))

			v1 := pbutil.Variant("k", "1")
			v11 := pbutil.Variant("k", "1", "k2", "1")
			v2 := pbutil.Variant("k", "2")
			MustApply(ctx, CombineMutations(
				insertTestResults(makeTestResults("inv0", "1-1", v1, pb.TestStatus_PASS, pb.TestStatus_FAIL)),
				insertTestResults(makeTestResults("inv0", "1-2", v11, pb.TestStatus_PASS)),
				insertTestResults(makeTestResults("inv1", "1-1", v1, pb.TestStatus_PASS)),
				insertTestResults(makeTestResults("inv1", "2-1", v2, pb.TestStatus_PASS)),
			)...)

			q.InvocationIDs = span.NewInvocationIDSet("inv0", "inv1")
			q.Predicate.Variant = &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_Contains{Contains: v1},
			}

			So(mustReadNames(q), ShouldResemble, []string{
				"invocations/inv0/tests/1-1/results/0",
				"invocations/inv0/tests/1-1/results/1",
				"invocations/inv0/tests/1-2/results/0",
				"invocations/inv1/tests/1-1/results/0",
			})
		})

		Convey(`Paging`, func() {
			trs := makeTestResults("inv1", "DoBaz", nil,
				pb.TestStatus_PASS,
				pb.TestStatus_FAIL,
				pb.TestStatus_FAIL,
				pb.TestStatus_PASS,
				pb.TestStatus_FAIL,
			)
			MustApply(ctx, insertTestResults(trs)...)

			mustReadPage := func(pageToken string, pageSize int, expected []*pb.TestResult) string {
				q2 := q
				q2.PageToken = pageToken
				q2.PageSize = pageSize
				actual, token := mustRead(q2)
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
					_, _, err := span.QueryTestResults(ctx, txn, q)
					So(err, ShouldHaveAppStatus, codes.InvalidArgument, "invalid page_token")
				})

				Convey(`From decoding`, func() {
					q.PageToken = "%%%"
					_, _, err := span.QueryTestResults(ctx, txn, q)
					So(err, ShouldHaveAppStatus, codes.InvalidArgument, "invalid page_token")
				})
			})
		})
	})
}
