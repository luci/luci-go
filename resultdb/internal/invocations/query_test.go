// Copyright 2021 The LUCI Authors.
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

package invocations

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInvsMatchingPredicate(t *testing.T) {
	Convey(`InvsMatchingPredicate`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		// Insert some Invocations.
		testutil.MustApply(ctx,
			insertInvocation("inv1", map[string]interface{}{
				"CommonTestIDPrefix": "ninja://browser_tests/",
			}),
			insertInvocation("inv2", map[string]interface{}{
				"CommonTestIDPrefix":     "",
				"TestResultVariantUnion": pbutil.Variant("k", "v"),
			}),
			insertInvocation("inv3", nil),
			insertInvocation("inv4", map[string]interface{}{
				"CommonTestIDPrefix":     "ninja://",
				"TestResultVariantUnion": pbutil.Variant("k", "v", "a", "b"),
			}),
			insertInvocation("inv5", nil),
		)
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		invIDs := make(IDSet, 5)
		for i := 1; i <= 5; i++ {
			invIDs.Add(ID(fmt.Sprintf("inv%d", i)))
		}

		Convey(`testIdRegexp with non literal prefix`, func() {
			predicate := &pb.TestResultPredicate{TestIdRegexp: ".*random_prefix"}
			ret, err := InvsMatchingPredicate(ctx, invIDs, predicate)
			So(err, ShouldBeNil)
			So(ret, ShouldResemble, NewIDSet("inv1", "inv2", "inv3", "inv4", "inv5"))
		})

		Convey(`unmatched testIdRegexp`, func() {
			predicate := &pb.TestResultPredicate{TestIdRegexp: "random_prefix"}
			ret, err := InvsMatchingPredicate(ctx, invIDs, predicate)
			So(err, ShouldBeNil)
			So(ret, ShouldResemble, NewIDSet("inv2"))
		})

		Convey(`matched testIdRegexp`, func() {
			predicate := &pb.TestResultPredicate{TestIdRegexp: "ninja://browser_.*abc"}
			ret, err := InvsMatchingPredicate(ctx, invIDs, predicate)
			So(err, ShouldBeNil)
			So(ret, ShouldResemble, NewIDSet("inv1", "inv2", "inv4"))
		})

		Convey(`matched variant`, func() {
			predicate := &pb.TestResultPredicate{
				Variant: &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Equals{
						Equals: pbutil.Variant("a", "b"),
					},
				},
			}
			ret, err := InvsMatchingPredicate(ctx, invIDs, predicate)
			So(err, ShouldBeNil)
			So(ret, ShouldResemble, NewIDSet("inv4"))
		})
	})
}
