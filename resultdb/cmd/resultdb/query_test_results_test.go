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
	"testing"

	durpb "github.com/golang/protobuf/ptypes/duration"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	"go.chromium.org/luci/resultdb/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateQueryTestResultsRequest(t *testing.T) {
	t.Parallel()
	Convey(`Valid`, t, func() {
		err := validateQueryTestResultsRequest(&pb.QueryTestResultsRequest{
			Predicate: &pb.TestResultPredicate{
				Invocation: &pb.InvocationPredicate{
					RootPredicate: &pb.InvocationPredicate_Name{Name: "invocations/x"},
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
					RootPredicate: &pb.InvocationPredicate_Name{Name: "xxxxxxxxxxxxx"},
				},
			},
		})
		So(err, ShouldErrLike, `predicate: invocation: name: does not match`)
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
					RootPredicate: &pb.InvocationPredicate_Name{Name: "invocations/a"},
				},
			},
		})
		So(err, ShouldBeNil)
		So(res.Groups, ShouldHaveLength, 3)

		g := res.Groups["invocations/a"]
		So(g.Invocation.Name, ShouldEqual, "invocations/a")
		So(g.Invocation.State, ShouldEqual, pb.Invocation_COMPLETED)
		So(g.TestResults, ShouldHaveLength, 2)
		So(g.TestResults[0].Name, ShouldEqual, "invocations/a/tests/A/results/0")
		So(g.TestResults[0].Status, ShouldEqual, pb.TestStatus_FAIL)
		So(g.TestResults[1].Name, ShouldEqual, "invocations/a/tests/A/results/1")
		So(g.TestResults[1].Status, ShouldEqual, pb.TestStatus_PASS)

		g = res.Groups["invocations/b"]
		So(g.Invocation.Name, ShouldEqual, "invocations/b")
		So(g.Invocation.State, ShouldEqual, pb.Invocation_COMPLETED)
		So(g.TestResults, ShouldHaveLength, 2)
		So(g.TestResults[0].Name, ShouldEqual, "invocations/b/tests/B/results/0")
		So(g.TestResults[0].Status, ShouldEqual, pb.TestStatus_CRASH)
		So(g.TestResults[1].Name, ShouldEqual, "invocations/b/tests/B/results/1")
		So(g.TestResults[1].Status, ShouldEqual, pb.TestStatus_PASS)

		g = res.Groups["invocations/c"]
		So(g.Invocation.Name, ShouldEqual, "invocations/c")
		So(g.Invocation.State, ShouldEqual, pb.Invocation_COMPLETED)
		So(g.TestResults, ShouldHaveLength, 1)
		So(g.TestResults[0].Name, ShouldEqual, "invocations/c/tests/C/results/0")
		So(g.TestResults[0].Status, ShouldEqual, pb.TestStatus_PASS)
	})
}
