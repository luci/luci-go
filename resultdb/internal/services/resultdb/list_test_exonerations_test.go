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

func TestValidateListTestExonerationsRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`Valid`, t, func(t *ftt.Test) {
		req := &pb.ListTestExonerationsRequest{Invocation: "invocations/inv", PageSize: 50}
		assert.Loosely(t, validateListTestExonerationsRequest(req), should.BeNil)
	})

	ftt.Run(`Invalid invocation`, t, func(t *ftt.Test) {
		req := &pb.ListTestExonerationsRequest{Invocation: "bad_name", PageSize: 50}
		assert.Loosely(t, validateListTestExonerationsRequest(req), should.ErrLike("invocation: does not match"))
	})

	ftt.Run(`Invalid page size`, t, func(t *ftt.Test) {
		req := &pb.ListTestExonerationsRequest{Invocation: "invocations/inv", PageSize: -50}
		assert.Loosely(t, validateListTestExonerationsRequest(req), should.ErrLike("page_size: negative"))
	})
}

func TestListTestExonerations(t *testing.T) {
	ftt.Run(`ListTestExonerations`, t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestExonerations},
			},
		})

		// Insert some TestExonerations.
		invID := invocations.ID("inv")
		testID := "ninja://chrome/test:foo_tests/BarTest.DoBaz"
		var0 := pbutil.Variant("k1", "v1", "k2", "v2")
		testutil.MustApply(ctx, t,
			insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:testrealm"}),
			insert.Invocation("invx", pb.Invocation_ACTIVE, map[string]any{"Realm": "secretproject:testrealm"}),
			spanutil.InsertMap("TestExonerations", map[string]any{
				"InvocationId":    invID,
				"TestId":          testID,
				"ExonerationId":   "0",
				"Variant":         var0,
				"VariantHash":     "deadbeef",
				"ExplanationHTML": spanutil.Compressed("broken"),
				"Reason":          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
			}),
			spanutil.InsertMap("TestExonerations", map[string]any{
				"InvocationId":  invID,
				"TestId":        testID,
				"ExonerationId": "1",
				"Variant":       pbutil.Variant(),
				"VariantHash":   "deadbeef",
				"Reason":        pb.ExonerationReason_UNEXPECTED_PASS,
			}),
			spanutil.InsertMap("TestExonerations", map[string]any{
				"InvocationId":  invID,
				"TestId":        testID,
				"ExonerationId": "2",
				"Variant":       pbutil.Variant(),
				"VariantHash":   "deadbeef",
				"Reason":        pb.ExonerationReason_UNEXPECTED_PASS,
			}),
		)

		all := []*pb.TestExoneration{
			{
				Name:            pbutil.TestExonerationName("inv", testID, "0"),
				TestId:          testID,
				Variant:         var0,
				VariantHash:     "deadbeef",
				ExonerationId:   "0",
				ExplanationHtml: "broken",
				Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
			},
			{
				Name:          pbutil.TestExonerationName("inv", testID, "1"),
				TestId:        testID,
				ExonerationId: "1",
				VariantHash:   "deadbeef",
				Reason:        pb.ExonerationReason_UNEXPECTED_PASS,
			},
			{
				Name:          pbutil.TestExonerationName("inv", testID, "2"),
				TestId:        testID,
				ExonerationId: "2",
				VariantHash:   "deadbeef",
				Reason:        pb.ExonerationReason_UNEXPECTED_PASS,
			},
		}
		srv := newTestResultDBService()

		t.Run(`Permission denied`, func(t *ftt.Test) {
			req := &pb.ListTestExonerationsRequest{Invocation: "invocations/invx"}
			_, err := srv.ListTestExonerations(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.testExonerations.list in realm of invocation invx"))
		})

		t.Run(`Basic`, func(t *ftt.Test) {
			req := &pb.ListTestExonerationsRequest{Invocation: "invocations/inv"}
			resp, err := srv.ListTestExonerations(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.NotBeNil)
			assert.Loosely(t, resp.TestExonerations, should.Match(all))
			assert.Loosely(t, resp.NextPageToken, should.BeEmpty)
		})

		t.Run(`With pagination`, func(t *ftt.Test) {
			req := &pb.ListTestExonerationsRequest{
				Invocation: "invocations/inv",
				PageSize:   1,
			}
			res, err := srv.ListTestExonerations(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.NotBeNil)
			assert.Loosely(t, res.TestExonerations, should.Match(all[:1]))
			assert.Loosely(t, res.NextPageToken, should.NotEqual(""))

			t.Run(`Next one`, func(t *ftt.Test) {
				req.PageToken = res.NextPageToken
				res, err = srv.ListTestExonerations(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.NotBeNil)
				assert.Loosely(t, res.TestExonerations, should.Match(all[1:2]))
				assert.Loosely(t, res.NextPageToken, should.NotEqual(""))
			})
			t.Run(`Next all`, func(t *ftt.Test) {
				req.PageToken = res.NextPageToken
				req.PageSize = 100
				res, err = srv.ListTestExonerations(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.NotBeNil)
				assert.Loosely(t, res.TestExonerations, should.Match(all[1:]))
				assert.Loosely(t, res.NextPageToken, should.BeEmpty)
			})
		})
	})
}
