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

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"

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
					Inclusions:    pb.InvocationPredicate_STABLE,
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

func TestReadInvocationIDsByTag(t *testing.T) {
	Convey(`TestReadInvocationIDsByTag`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		tagAB := pbutil.StringPair("a", "b")
		tagAC := pbutil.StringPair("a", "c")
		testutil.MustApply(ctx,
			span.InsertMap("InvocationsByTag", map[string]interface{}{
				"TagId":        span.TagRowID(tagAB),
				"InvocationId": span.InvocationID("inv0"),
			}),
			span.InsertMap("InvocationsByTag", map[string]interface{}{
				"TagId":        span.TagRowID(tagAB),
				"InvocationId": span.InvocationID("inv1"),
			}),
			span.InsertMap("InvocationsByTag", map[string]interface{}{
				"TagId":        span.TagRowID(tagAC),
				"InvocationId": span.InvocationID("inv3"),
			}),
		)

		txn := span.Client(ctx).ReadOnlyTransaction()
		defer txn.Close()

		Convey(`works`, func() {
			invIDs, err := span.ReadInvocationIDsByTag(ctx, txn, tagAB, 0)
			So(err, ShouldBeNil)
			So(invIDs, ShouldResemble, []span.InvocationID{"inv0", "inv1"})
		})

		Convey(`limit`, func() {
			_, err := span.ReadInvocationIDsByTag(ctx, txn, tagAB, 1)
			So(err, ShouldErrLike, `more than 1 invocations have tag "a:b"`)
			So(span.TooManyInvocationsTag.In(err), ShouldBeTrue)
		})
	})
}
