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

package invocations

import (
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRead(t *testing.T) {
	Convey(`Read`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		start := testclock.TestRecentTimeUTC

		// Insert some Invocations.
		testutil.MustApply(ctx,
			insertInvocation("including", map[string]interface{}{
				"State":      pb.Invocation_ACTIVE,
				"CreateTime": start,
				"Deadline":   start.Add(time.Hour),
			}),
			insertInvocation("included0", nil),
			insertInvocation("included1", nil),
			insertInclusion("including", "included0"),
			insertInclusion("including", "included1"),
		)

		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		// Fetch back the top-level Invocation.
		inv, err := Read(ctx, "including")
		So(err, ShouldBeNil)
		So(inv, ShouldResembleProto, &pb.Invocation{
			Name:                "invocations/including",
			State:               pb.Invocation_ACTIVE,
			CreateTime:          pbutil.MustTimestampProto(start),
			Deadline:            pbutil.MustTimestampProto(start.Add(time.Hour)),
			IncludedInvocations: []string{"invocations/included0", "invocations/included1"},
		})
	})
}

func TestReadBatch(t *testing.T) {
	Convey(`TestReadBatch`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		testutil.MustApply(ctx,
			insertInvocation("inv0", nil),
			insertInvocation("inv1", nil),
			insertInvocation("inv2", nil),
		)

		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		Convey(`One name`, func() {
			invs, err := ReadBatch(ctx, NewIDSet("inv1"))
			So(err, ShouldBeNil)
			So(invs, ShouldHaveLength, 1)
			So(invs, ShouldContainKey, ID("inv1"))
			So(invs["inv1"].Name, ShouldEqual, "invocations/inv1")
			So(invs["inv1"].State, ShouldEqual, pb.Invocation_FINALIZED)
		})

		Convey(`Two names`, func() {
			invs, err := ReadBatch(ctx, NewIDSet("inv0", "inv1"))
			So(err, ShouldBeNil)
			So(invs, ShouldHaveLength, 2)
			So(invs, ShouldContainKey, ID("inv0"))
			So(invs, ShouldContainKey, ID("inv1"))
			So(invs["inv0"].Name, ShouldEqual, "invocations/inv0")
			So(invs["inv0"].State, ShouldEqual, pb.Invocation_FINALIZED)
		})

		Convey(`Not found`, func() {
			_, err := ReadBatch(ctx, NewIDSet("inv0", "x"))
			So(err, ShouldErrLike, `invocations/x not found`)
		})
	})
}

func TestQueryRealms(t *testing.T) {
	Convey(`TestQueryRealms`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		Convey(`Works`, func() {
			testutil.MustApply(ctx,
				insertInvocation("inv0", map[string]interface{}{"Realm": "0"}),
				insertInvocation("inv1", map[string]interface{}{"Realm": "1"}),
				insertInvocation("inv2", map[string]interface{}{"Realm": "2"}),
			)

			realms, err := QueryRealms(span.Single(ctx), NewIDSet("inv0", "inv1", "inv2"))
			So(err, ShouldBeNil)
			So(realms, ShouldResemble, map[ID]string{
				"inv0": "0",
				"inv1": "1",
				"inv2": "2",
			})
		})
		Convey(`Valid with missing invocation `, func() {
			testutil.MustApply(ctx,
				insertInvocation("inv0", map[string]interface{}{"Realm": "0"}),
			)

			realms, err := QueryRealms(span.Single(ctx), NewIDSet("inv0", "inv1"))
			So(err, ShouldBeNil)
			So(realms, ShouldResemble, map[ID]string{
				"inv0": "0",
			})
		})
	})
}

func TestReadRealms(t *testing.T) {
	Convey(`TestReadRealms`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		Convey(`Works`, func() {
			testutil.MustApply(ctx,
				insertInvocation("inv0", map[string]interface{}{"Realm": "0"}),
				insertInvocation("inv1", map[string]interface{}{"Realm": "1"}),
				insertInvocation("inv2", map[string]interface{}{"Realm": "2"}),
			)

			realms, err := ReadRealms(span.Single(ctx), NewIDSet("inv0", "inv1", "inv2"))
			So(err, ShouldBeNil)
			So(realms, ShouldResemble, map[ID]string{
				"inv0": "0",
				"inv1": "1",
				"inv2": "2",
			})
		})
		Convey(`NotFound`, func() {
			testutil.MustApply(ctx,
				insertInvocation("inv0", map[string]interface{}{"Realm": "0"}),
			)

			_, err := ReadRealms(span.Single(ctx), NewIDSet("inv0", "inv1"))
			So(err, ShouldHaveAppStatus, codes.NotFound, "invocations/inv1 not found")
		})
	})
}
