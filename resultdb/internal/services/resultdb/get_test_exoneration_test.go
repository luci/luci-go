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
	"testing"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateGetTestExonerationRequest(t *testing.T) {
	t.Parallel()
	Convey(`ValidateGetTestExonerationRequest`, t, func() {
		Convey(`Valid`, func() {
			req := &pb.GetTestExonerationRequest{Name: "invocations/a/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/exonerations/id"}
			So(validateGetTestExonerationRequest(req), ShouldBeNil)
		})

		Convey(`Invalid name`, func() {
			req := &pb.GetTestExonerationRequest{}
			So(validateGetTestExonerationRequest(req), ShouldErrLike, "unspecified")
		})
	})
}

func TestGetTestExoneration(t *testing.T) {
	Convey(`GetTestExoneration`, t, func() {
		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermGetTestExoneration},
			},
		}
		ctx := auth.WithState(testutil.SpannerTestContext(t), authState)

		srv := newTestResultDBService()

		invID := invocations.ID("inv_0")
		// Insert a TestExoneration.
		testutil.MustApply(ctx,
			insert.Invocation("inv_0", pb.Invocation_ACTIVE, nil),
			spanutil.InsertMap("TestExonerations", map[string]any{
				"InvocationId":    invID,
				"TestId":          "ninja://chrome/test:foo_tests/BarTest.DoBaz",
				"ExonerationId":   "id",
				"Variant":         pbutil.Variant("k1", "v1", "k2", "v2"),
				"VariantHash":     "deadbeef",
				"ExplanationHTML": spanutil.Compressed("broken"),
				"Reason":          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
			}))

		req := &pb.GetTestExonerationRequest{Name: "invocations/inv_0/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/exonerations/id"}

		Convey(`Invocation does not exist`, func() {
			req := &pb.GetTestExonerationRequest{Name: "invocations/inv_notexists/tests/test/exonerations/id"}

			tr, err := srv.GetTestExoneration(ctx, req)
			So(tr, ShouldBeNil)
			So(err, ShouldBeRPCNotFound, "invocations/inv_notexists not found")
		})
		Convey(`Permission denied`, func() {
			testutil.MustApply(ctx, spanutil.UpdateMap("Invocations", map[string]any{
				"InvocationId": invocations.ID("inv_0"),
				"Realm":        "secretproject:testrealm",
			}))

			tr, err := srv.GetTestExoneration(ctx, req)
			So(tr, ShouldBeNil)
			So(err, ShouldBeRPCPermissionDenied, "caller does not have permission resultdb.testExonerations.get in realm of invocation inv_0")
		})
		Convey("Valid", func() {
			tr, err := srv.GetTestExoneration(ctx, req)
			So(err, ShouldBeNil)
			So(tr, ShouldResembleProto, &pb.TestExoneration{
				Name:            "invocations/inv_0/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/exonerations/id",
				ExonerationId:   "id",
				TestId:          "ninja://chrome/test:foo_tests/BarTest.DoBaz",
				Variant:         pbutil.Variant("k1", "v1", "k2", "v2"),
				VariantHash:     "deadbeef",
				ExplanationHtml: "broken",
				Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
			})
		})
	})
}
