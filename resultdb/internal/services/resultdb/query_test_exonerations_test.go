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

package resultdb

import (
	"sort"
	"testing"

	durpb "github.com/golang/protobuf/ptypes/duration"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateQueryTestExonerationsRequest(t *testing.T) {
	t.Parallel()
	Convey(`Valid`, t, func() {
		err := validateQueryTestExonerationsRequest(&pb.QueryTestExonerationsRequest{
			Invocations:  []string{"invocations/x"},
			PageSize:     50,
			MaxStaleness: &durpb.Duration{Seconds: 60},
		})
		So(err, ShouldBeNil)
	})

	Convey(`invalid predicate`, t, func() {
		err := validateQueryTestExonerationsRequest(&pb.QueryTestExonerationsRequest{
			Invocations: []string{"x"},
		})
		So(err, ShouldErrLike, `invocations: "x": does not match`)
	})
}

func TestQueryTestExonerations(t *testing.T) {
	Convey(`QueryTestExonerations`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		insertInv := insert.FinalizedInvocationWithInclusions
		insertEx := insert.TestExonerations
		testutil.MustApply(ctx, testutil.CombineMutations(
			insertInv("a", "b"),
			insertInv("b", "c"),
			insertInv("c"),
			insertEx("a", "A", pbutil.Variant("v", "a"), 2),
			insertEx("c", "C", pbutil.Variant("v", "c"), 1),
		)...)

		srv := newTestResultDBService()
		res, err := srv.QueryTestExonerations(ctx, &pb.QueryTestExonerationsRequest{
			Invocations: []string{"invocations/a"},
		})
		So(err, ShouldBeNil)
		actual := res.TestExonerations
		sort.Slice(actual, func(i, j int) bool {
			return actual[i].Name < actual[j].Name
		})
		So(actual, ShouldResembleProto, []*pb.TestExoneration{
			{
				Name:            "invocations/a/tests/A/exonerations/0",
				TestId:          "A",
				Variant:         pbutil.Variant("v", "a"),
				ExonerationId:   "0",
				ExplanationHtml: "explanation 0",
			},
			{
				Name:            "invocations/a/tests/A/exonerations/1",
				TestId:          "A",
				Variant:         pbutil.Variant("v", "a"),
				ExonerationId:   "1",
				ExplanationHtml: "explanation 1",
			},
			{
				Name:            "invocations/c/tests/C/exonerations/0",
				TestId:          "C",
				Variant:         pbutil.Variant("v", "c"),
				ExonerationId:   "0",
				ExplanationHtml: "explanation 0",
			},
		})
	})
}
