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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestValidateGetTestExonerationRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`ValidateGetTestExonerationRequest`, t, func(t *ftt.Test) {
		t.Run(`Valid`, func(t *ftt.Test) {
			req := &pb.GetTestExonerationRequest{Name: "invocations/a/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/exonerations/id"}
			assert.Loosely(t, validateGetTestExonerationRequest(req), should.BeNil)
		})

		t.Run(`Invalid name`, func(t *ftt.Test) {
			req := &pb.GetTestExonerationRequest{}
			assert.Loosely(t, validateGetTestExonerationRequest(req), should.ErrLike("unspecified"))
		})
	})
}

func TestGetTestExoneration(t *testing.T) {
	ftt.Run(`GetTestExoneration`, t, func(t *ftt.Test) {
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
		testutil.MustApply(ctx, t,
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

		t.Run(`Invocation does not exist`, func(t *ftt.Test) {
			req := &pb.GetTestExonerationRequest{Name: "invocations/inv_notexists/tests/test/exonerations/id"}

			tr, err := srv.GetTestExoneration(ctx, req)
			assert.Loosely(t, tr, should.BeNil)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("invocations/inv_notexists not found"))
		})
		t.Run(`Permission denied`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t, spanutil.UpdateMap("Invocations", map[string]any{
				"InvocationId": invocations.ID("inv_0"),
				"Realm":        "secretproject:testrealm",
			}))

			tr, err := srv.GetTestExoneration(ctx, req)
			assert.Loosely(t, tr, should.BeNil)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.testExonerations.get in realm of invocation inv_0"))
		})
		t.Run("Valid", func(t *ftt.Test) {
			tr, err := srv.GetTestExoneration(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tr, should.Match(&pb.TestExoneration{
				Name:            "invocations/inv_0/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/exonerations/id",
				ExonerationId:   "id",
				TestId:          "ninja://chrome/test:foo_tests/BarTest.DoBaz",
				Variant:         pbutil.Variant("k1", "v1", "k2", "v2"),
				VariantHash:     "deadbeef",
				ExplanationHtml: "broken",
				Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
			}))
		})
	})
}
