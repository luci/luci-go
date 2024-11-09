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

package recorder

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateUpdateIncludedInvocationsRequest(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateUpdateIncludedInvocationsRequest`, t, func(t *ftt.Test) {
		t.Run(`Valid`, func(t *ftt.Test) {
			err := validateUpdateIncludedInvocationsRequest(&pb.UpdateIncludedInvocationsRequest{
				IncludingInvocation: "invocations/a",
				AddInvocations:      []string{"invocations/b"},
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`Invalid including_invocation`, func(t *ftt.Test) {
			err := validateUpdateIncludedInvocationsRequest(&pb.UpdateIncludedInvocationsRequest{
				IncludingInvocation: "x",
				AddInvocations:      []string{"invocations/b"},
			})
			assert.Loosely(t, err, should.ErrLike(`including_invocation: does not match`))
		})
		t.Run(`Invalid add_invocations`, func(t *ftt.Test) {
			err := validateUpdateIncludedInvocationsRequest(&pb.UpdateIncludedInvocationsRequest{
				IncludingInvocation: "invocations/a",
				AddInvocations:      []string{"x"},
				RemoveInvocations:   []string{"invocations/c"},
			})
			assert.Loosely(t, err, should.ErrLike(`add_invocations: "x": does not match`))
		})
		t.Run(`Attempt to remove invocations`, func(t *ftt.Test) {
			err := validateUpdateIncludedInvocationsRequest(&pb.UpdateIncludedInvocationsRequest{
				IncludingInvocation: "invocations/a",
				AddInvocations:      []string{"invocations/b"},
				RemoveInvocations:   []string{"invocations/x"},
			})
			assert.Loosely(t, err, should.ErrLike(`remove_invocations: invocation removal has been deprecated and is not permitted`))
		})
		t.Run(`Include itself`, func(t *ftt.Test) {
			err := validateUpdateIncludedInvocationsRequest(&pb.UpdateIncludedInvocationsRequest{
				IncludingInvocation: "invocations/a",
				AddInvocations:      []string{"invocations/a"},
			})
			assert.Loosely(t, err, should.ErrLike(`cannot include itself`))
		})
	})
}

func TestUpdateIncludedInvocations(t *testing.T) {
	ftt.Run(`TestIncludedInvocations`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, _ = tq.TestingContext(ctx, nil)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: permIncludeInvocation},
			},
		})
		recorder := newTestRecorderServer()

		token, err := generateInvocationToken(ctx, "including")
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		assertIncluded := func(includedInvID invocations.ID) {
			var throwAway invocations.ID
			testutil.MustReadRow(ctx, t, "IncludedInvocations", invocations.InclusionKey("including", includedInvID), map[string]any{
				"IncludedInvocationID": &throwAway,
			})
		}
		assertNotIncluded := func(includedInvID invocations.ID) {
			var throwAway invocations.ID
			testutil.MustNotFindRow(ctx, t, "IncludedInvocations", invocations.InclusionKey("including", includedInvID), map[string]any{
				"IncludedInvocationID": &throwAway,
			})
		}

		t.Run(`Invalid request`, func(t *ftt.Test) {
			_, err := recorder.UpdateIncludedInvocations(ctx, &pb.UpdateIncludedInvocationsRequest{})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`bad request: including_invocation: unspecified`))
		})

		t.Run(`With valid request`, func(t *ftt.Test) {
			req := &pb.UpdateIncludedInvocationsRequest{
				IncludingInvocation: "invocations/including",
				AddInvocations: []string{
					"invocations/included",
					"invocations/included2",
				},
			}

			t.Run(`No including invocation`, func(t *ftt.Test) {
				testutil.MustApply(ctx, t,
					insert.Invocation("included", pb.Invocation_FINALIZED, map[string]any{"Realm": "testproject:testrealm"}),
					insert.Invocation("included2", pb.Invocation_FINALIZED, map[string]any{"Realm": "testproject:testrealm"}),
				)
				_, err := recorder.UpdateIncludedInvocations(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike(`invocations/including not found`))
			})

			t.Run(`With existing inclusion`, func(t *ftt.Test) {
				testutil.MustApply(ctx, t,
					insert.Invocation("including", pb.Invocation_ACTIVE, nil),
					insert.Invocation("existinginclusion", pb.Invocation_FINALIZED, map[string]any{"Realm": "testproject:testrealm"}),
				)
				_, err := recorder.UpdateIncludedInvocations(ctx, &pb.UpdateIncludedInvocationsRequest{
					IncludingInvocation: "invocations/including",
					AddInvocations:      []string{"invocations/existinginclusion"},
				})
				assert.Loosely(t, err, should.BeNil)
				assertIncluded("existinginclusion")

				t.Run(`No included invocation`, func(t *ftt.Test) {
					_, err := recorder.UpdateIncludedInvocations(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
					assert.Loosely(t, err, should.ErrLike(`invocations/included`))
				})

				t.Run(`Leaking disallowed`, func(t *ftt.Test) {
					testutil.MustApply(ctx, t,
						insert.Invocation("included", pb.Invocation_FINALIZED, map[string]any{"Realm": "testproject:testrealm"}),
						insert.Invocation("included2", pb.Invocation_FINALIZED, map[string]any{"Realm": "testproject:secretrealm"}),
					)

					_, err := recorder.UpdateIncludedInvocations(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.Loosely(t, err, should.ErrLike(`caller does not have permission resultdb.invocations.include in realm of invocation included2`))
					assertNotIncluded("included2")
				})

				t.Run(`Success - idempotent`, func(t *ftt.Test) {
					testutil.MustApply(ctx, t,
						insert.Invocation("included", pb.Invocation_FINALIZED, map[string]any{"Realm": "testproject:testrealm"}),
						insert.Invocation("included2", pb.Invocation_FINALIZED, map[string]any{"Realm": "testproject:testrealm"}),
					)

					_, err := recorder.UpdateIncludedInvocations(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assertIncluded("included")
					assertIncluded("included2")
					assertIncluded("existinginclusion")

					_, err = recorder.UpdateIncludedInvocations(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assertIncluded("included")
					assertIncluded("included2")
					assertIncluded("existinginclusion")
				})
			})
		})
	})
}
