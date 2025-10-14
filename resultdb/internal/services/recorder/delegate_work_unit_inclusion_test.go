// Copyright 2025 The LUCI Authors.
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

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestDelegateWorkUnitInclusion(t *testing.T) {
	ftt.Run("DelegateWorkUnitInclusion", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: permIncludeWorkUnit},
			},
		})

		// Setup a full HTTP server to retrieve response headers.
		server := &prpctest.Server{}
		pb.RegisterRecorderServer(server, newTestRecorderServer())
		server.Start(ctx)
		defer server.Close()
		client, err := server.NewClient()
		assert.Loosely(t, err, should.BeNil)
		recorder := pb.NewRecorderPRPCClient(client)

		rootInvID := rootinvocations.ID("root-inv-id")
		workUnitID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "wu-parent",
		}

		// Insert a root invocation and the parent work unit.
		rootInv := rootinvocations.NewBuilder(rootInvID).Build()
		wu := workunits.NewBuilder(rootInvID, workUnitID.WorkUnitID).WithFinalizationState(pb.WorkUnit_ACTIVE).Build()
		testutil.MustApply(ctx, t, insert.RootInvocationWithRootWorkUnit(rootInv)...)
		testutil.MustApply(ctx, t, insert.WorkUnit(wu)...)

		// Generate an update token for the work unit.
		updateToken, err := generateWorkUnitUpdateToken(ctx, workUnitID)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, updateToken))

		// A basic valid request.
		req := &pb.DelegateWorkUnitInclusionRequest{
			WorkUnit: workUnitID.Name(),
			Realm:    "testproject:testrealm",
		}

		t.Run("invalid request", func(t *ftt.Test) {
			// Request validation exhaustively tested in TestVerifyDelegateWorkUnitInclusionPermissions.
			// These tests only exist to ensure that method is called.
			t.Run("invalid work unit name", func(t *ftt.Test) {
				badReq := &pb.DelegateWorkUnitInclusionRequest{
					WorkUnit: "invalid name",
					Realm:    "testproject:testrealm",
				}

				_, err := recorder.DelegateWorkUnitInclusion(ctx, badReq)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("work_unit: does not match"))
			})

			t.Run("missing realm", func(t *ftt.Test) {
				badReq := &pb.DelegateWorkUnitInclusionRequest{
					WorkUnit: workUnitID.Name(),
					Realm:    "",
				}

				_, err := recorder.DelegateWorkUnitInclusion(ctx, badReq)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("realm: unspecified"))
			})
		})

		t.Run("permission denied", func(t *ftt.Test) {
			// Request authorisation exhaustively tested in TestVerifyDelegateWorkUnitInclusionPermissions.
			// This test case only exists to verify that method is called.
			t.Run("no include work unit permission", func(t *ftt.Test) {
				badReq := &pb.DelegateWorkUnitInclusionRequest{
					WorkUnit: workUnitID.Name(),
					Realm:    "secretproject:testrealm",
				}

				_, err := recorder.DelegateWorkUnitInclusion(ctx, badReq)
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike(`caller does not have permission "resultdb.workUnits.include"`))
			})
		})

		t.Run("work unit not active", func(t *ftt.Test) {
			testutil.MustApply(ctx, t, spanutil.UpdateMap("WorkUnits", map[string]any{
				"RootInvocationShardId": workUnitID.RootInvocationShardID(),
				"WorkUnitId":            workUnitID.WorkUnitID,
				"State":                 pb.WorkUnit_FINALIZING,
			}))

			_, err = recorder.DelegateWorkUnitInclusion(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
			assert.That(t, err, should.ErrLike("work unit \"rootInvocations/root-inv-id/workUnits/wu-parent\" is not active"))
		})

		t.Run("end to end success", func(t *ftt.Test) {
			var headers metadata.MD
			res, err := recorder.DelegateWorkUnitInclusion(ctx, req, grpc.Header(&headers))
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, res, should.Match(&pb.DelegateWorkUnitInclusionResponse{}))

			// Check for the new inclusion token in headers.
			token := headers.Get(pb.InclusionTokenMetadataKey)
			assert.Loosely(t, token, should.HaveLength(1))
			assert.Loosely(t, token[0], should.NotBeEmpty)

			// Verify the token is valid.
			gotRealm, err := validateWorkUnitInclusionTokenForState(ctx, token[0], workUnitInclusionTokenState(workUnitID))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gotRealm, should.Equal("testproject:testrealm"))
		})
	})
}

func TestVerifyDelegateWorkUnitInclusionPermissions(t *testing.T) {
	t.Parallel()

	ftt.Run("VerifyDelegateWorkUnitInclusionPermissions", t, func(t *ftt.Test) {
		ctx := testutil.TestingContext()
		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "project:realm", Permission: permIncludeWorkUnit},
			},
		}
		ctx = auth.WithState(ctx, authState)

		workUnitID := workunits.ID{
			RootInvocationID: "root-inv-id",
			WorkUnitID:       "work-unit-id",
		}
		updateToken, err := generateWorkUnitUpdateToken(ctx, workUnitID)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, updateToken))

		request := &pb.DelegateWorkUnitInclusionRequest{
			WorkUnit: workUnitID.Name(),
			Realm:    "project:realm",
		}

		t.Run("valid", func(t *ftt.Test) {
			err := verifyDelegateWorkUnitInclusionPermissions(ctx, request)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("invalid work unit name", func(t *ftt.Test) {
			request.WorkUnit = "invalid"
			err := verifyDelegateWorkUnitInclusionPermissions(ctx, request)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("work_unit: does not match"))
		})

		t.Run("missing update token", func(t *ftt.Test) {
			ctx := metadata.NewIncomingContext(ctx, metadata.MD{})
			err := verifyDelegateWorkUnitInclusionPermissions(ctx, request)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.Unauthenticated))
			assert.Loosely(t, err, should.ErrLike("missing update-token metadata value"))
		})

		t.Run("invalid update token", func(t *ftt.Test) {
			ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))
			err := verifyDelegateWorkUnitInclusionPermissions(ctx, request)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("invalid update token"))
		})

		t.Run("too many update tokens", func(t *ftt.Test) {
			ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, updateToken, pb.UpdateTokenMetadataKey, updateToken))
			err := verifyDelegateWorkUnitInclusionPermissions(ctx, request)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("expected exactly one update-token metadata value, got 2"))
		})

		t.Run("unspecified realm", func(t *ftt.Test) {
			request.Realm = ""
			err := verifyDelegateWorkUnitInclusionPermissions(ctx, request)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("realm: unspecified"))
		})

		t.Run("invalid realm", func(t *ftt.Test) {
			request.Realm = "invalid:"
			err := verifyDelegateWorkUnitInclusionPermissions(ctx, request)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("realm: bad global realm name"))
		})

		t.Run("permission denied", func(t *ftt.Test) {
			authState.IdentityPermissions = nil
			err := verifyDelegateWorkUnitInclusionPermissions(ctx, request)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike(`caller does not have permission "resultdb.workUnits.include"`))
		})
	})
}

func TestWorkUnitInclusionToken(t *testing.T) {
	t.Parallel()

	ftt.Run("WorkUnitInclusionToken", t, func(t *ftt.Test) {
		ctx := testutil.TestingContext()
		id := workunits.ID{
			RootInvocationID: "root-inv-id",
			WorkUnitID:       "work-unit-id",
		}
		realm := "project:realm"

		t.Run("round-trip", func(t *ftt.Test) {
			token, err := generateWorkUnitInclusionToken(ctx, id, realm)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, token, should.NotBeEmpty)

			gotRealm, err := validateWorkUnitInclusionTokenForState(ctx, token, workUnitInclusionTokenState(id))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gotRealm, should.Equal(realm))
		})

		t.Run("invalid token", func(t *ftt.Test) {
			token, err := generateWorkUnitInclusionToken(ctx, id, realm)
			assert.Loosely(t, err, should.BeNil)

			t.Run("wrong root invocation ID", func(t *ftt.Test) {
				wrongID := workunits.ID{
					RootInvocationID: "wrong-root-inv-id",
					WorkUnitID:       "work-unit-id",
				}
				_, err := validateWorkUnitInclusionTokenForState(ctx, token, workUnitInclusionTokenState(wrongID))
				assert.Loosely(t, err, should.NotBeNil)
			})

			t.Run("wrong work unit ID", func(t *ftt.Test) {
				wrongID := workunits.ID{
					RootInvocationID: "root-inv-id",
					WorkUnitID:       "wrong-work-unit-id",
				}
				_, err := validateWorkUnitInclusionTokenForState(ctx, token, workUnitInclusionTokenState(wrongID))
				assert.Loosely(t, err, should.NotBeNil)
			})
		})

		t.Run("workUnitInclusionTokenState", func(t *ftt.Test) {
			assert.Loosely(t, workUnitInclusionTokenState(id), should.Equal(`"root-inv-id";"work-unit-id";inclusion-only`))
		})

		t.Run("extractInclusionOrUpdateToken", func(t *ftt.Test) {
			inclusionToken, err := generateWorkUnitInclusionToken(ctx, id, realm)
			assert.Loosely(t, err, should.BeNil)

			updateToken, err := generateWorkUnitUpdateToken(ctx, id)
			assert.Loosely(t, err, should.BeNil)

			t.Run("only inclusion token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.InclusionTokenMetadataKey, inclusionToken))
				inc, upd, err := extractInclusionOrUpdateToken(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, inc, should.Equal(inclusionToken))
				assert.Loosely(t, upd, should.BeEmpty)
			})
			t.Run("only update token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, updateToken))
				inc, upd, err := extractInclusionOrUpdateToken(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, inc, should.BeEmpty)
				assert.Loosely(t, upd, should.Equal(updateToken))
			})
			t.Run("both tokens", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.InclusionTokenMetadataKey, inclusionToken, pb.UpdateTokenMetadataKey, updateToken))
				_, _, err := extractInclusionOrUpdateToken(ctx)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("cannot specify both update-token and inclusion-token metadata values in the request"))
			})
			t.Run("no tokens", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.MD{})
				_, _, err := extractInclusionOrUpdateToken(ctx)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.Unauthenticated))
				assert.Loosely(t, err, should.ErrLike("expected either update-token or inclusion-token metadata value in the request"))
			})
			t.Run("multiple inclusion tokens", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.InclusionTokenMetadataKey, inclusionToken, pb.InclusionTokenMetadataKey, inclusionToken))
				_, _, err := extractInclusionOrUpdateToken(ctx)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("expected exactly one inclusion-token metadata value, got 2"))
			})
			t.Run("multiple update tokens", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, updateToken, pb.UpdateTokenMetadataKey, updateToken))
				_, _, err := extractInclusionOrUpdateToken(ctx)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("expected exactly one update-token metadata value, got 2"))
			})
		})
	})
}
