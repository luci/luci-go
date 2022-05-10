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

package exonerations

import (
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
)

func TestQueryTestExonerations(t *testing.T) {
	Convey(`QueryTestExonerations`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		testutil.MustApply(ctx, testutil.CombineMutations(
			insert.FinalizedInvocationWithInclusions("a", nil),
			insert.FinalizedInvocationWithInclusions("b", nil),
			insert.TestExonerations("a", "A", pbutil.Variant("v", "a"), pb.ExonerationReason_OCCURS_ON_OTHER_CLS, pb.ExonerationReason_NOT_CRITICAL),
			insert.TestExonerations("b", "C", pbutil.Variant("v", "c"), pb.ExonerationReason_UNEXPECTED_PASS),
		)...)

		q := &Query{
			InvocationIDs: invocations.NewIDSet("a", "b"),
			PageSize:      100,
		}
		actual, _, err := q.Fetch(span.Single(ctx))
		So(err, ShouldBeNil)
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
				VariantHash:     pbutil.VariantHash(pbutil.Variant("v", "a")),
				Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
			},
			{
				Name:            "invocations/a/tests/A/exonerations/1",
				TestId:          "A",
				Variant:         pbutil.Variant("v", "a"),
				ExonerationId:   "1",
				ExplanationHtml: "explanation 1",
				VariantHash:     pbutil.VariantHash(pbutil.Variant("v", "a")),
				Reason:          pb.ExonerationReason_NOT_CRITICAL,
			},
			{
				Name:            "invocations/b/tests/C/exonerations/0",
				TestId:          "C",
				Variant:         pbutil.Variant("v", "c"),
				ExonerationId:   "0",
				ExplanationHtml: "explanation 0",
				VariantHash:     pbutil.VariantHash(pbutil.Variant("v", "c")),
				Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
			},
		})
	})
}
