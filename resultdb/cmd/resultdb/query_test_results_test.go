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

package main

import (
	"sort"
	"testing"

	durpb "github.com/golang/protobuf/ptypes/duration"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	"go.chromium.org/luci/resultdb/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateQueryRequest(t *testing.T) {
	t.Parallel()
	Convey(`invalid max staleness`, t, func() {
		err := validateQueryRequest(&pb.QueryTestResultsRequest{
			Predicate: &pb.TestResultPredicate{
				Invocation: &pb.InvocationPredicate{Names: []string{"invocations/x"}},
			},
			MaxStaleness: &durpb.Duration{Seconds: -1},
		})
		So(err, ShouldErrLike, `max_staleness: must between 0 and 30m, inclusive`)
	})

	Convey(`invalid page size`, t, func() {
		err := validateQueryRequest(&pb.QueryTestResultsRequest{
			Predicate: &pb.TestResultPredicate{
				Invocation: &pb.InvocationPredicate{Names: []string{"invocations/x"}},
			},
			PageSize: -1,
		})
		So(err, ShouldErrLike, `page_size: negative`)
	})
}

func TestValidateQueryTestResultsRequest(t *testing.T) {
	t.Parallel()
	Convey(`Valid`, t, func() {
		err := validateQueryTestResultsRequest(&pb.QueryTestResultsRequest{
			Predicate: &pb.TestResultPredicate{
				Invocation: &pb.InvocationPredicate{
					Names: []string{"invocations/x"},
				},
			},
			PageSize:     50,
			MaxStaleness: &durpb.Duration{Seconds: 60},
		})
		So(err, ShouldBeNil)
	})

	Convey(`invalid predicate`, t, func() {
		err := validateQueryTestResultsRequest(&pb.QueryTestResultsRequest{
			Predicate: &pb.TestResultPredicate{
				Invocation: &pb.InvocationPredicate{
					Names: []string{"x"},
				},
			},
		})
		So(err, ShouldErrLike, `predicate: invocation: name "x": does not match`)
	})
}

func TestQueryTestResults(t *testing.T) {
	Convey(`QueryTestResults`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		insertInv := testutil.InsertInvocationWithInclusions
		testutil.MustApply(ctx, testutil.CombineMutations(
			insertInv("a", "b"),
			insertInv("b", "c"),
			insertInv("c"),
			testutil.InsertTestResults(testutil.MakeTestResults("a", "A", pb.TestStatus_FAIL, pb.TestStatus_PASS)),
			testutil.InsertTestResults(testutil.MakeTestResults("b", "B", pb.TestStatus_CRASH, pb.TestStatus_PASS)),
			testutil.InsertTestResults(testutil.MakeTestResults("c", "C", pb.TestStatus_PASS)),
		)...)

		srv := &resultDBServer{}
		res, err := srv.QueryTestResults(ctx, &pb.QueryTestResultsRequest{
			Predicate: &pb.TestResultPredicate{
				Invocation: &pb.InvocationPredicate{
					Names: []string{"invocations/a"},
				},
			},
		})
		So(err, ShouldBeNil)
		So(res.TestResults, ShouldHaveLength, 5)

		sort.Slice(res.TestResults, func(i, j int) bool {
			return res.TestResults[i].Name < res.TestResults[j].Name
		})

		So(res.TestResults[0].Name, ShouldEqual, "invocations/a/tests/A/results/0")
		So(res.TestResults[0].Status, ShouldEqual, pb.TestStatus_FAIL)

		So(res.TestResults[1].Name, ShouldEqual, "invocations/a/tests/A/results/1")
		So(res.TestResults[1].Status, ShouldEqual, pb.TestStatus_PASS)

		So(res.TestResults[2].Name, ShouldEqual, "invocations/b/tests/B/results/0")
		So(res.TestResults[2].Status, ShouldEqual, pb.TestStatus_CRASH)

		So(res.TestResults[3].Name, ShouldEqual, "invocations/b/tests/B/results/1")
		So(res.TestResults[3].Status, ShouldEqual, pb.TestStatus_PASS)

		So(res.TestResults[4].Name, ShouldEqual, "invocations/c/tests/C/results/0")
		So(res.TestResults[4].Status, ShouldEqual, pb.TestStatus_PASS)
	})
}
