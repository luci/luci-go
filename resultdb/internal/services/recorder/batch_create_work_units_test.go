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
	"fmt"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestBatchCreateWorkUnits(t *testing.T) {
	ftt.Run(`TestBatchCreateWorkUnits`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.
		err := config.SetServiceConfigForTesting(ctx, config.CreatePlaceHolderServiceConfig())
		assert.NoErr(t, err)

		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:@root", Permission: permCreateWorkUnitWithReservedID},
				{Realm: "testproject:@root", Permission: permSetWorkUnitProducerResource},
			},
		}
		ctx = auth.WithState(ctx, authState)

		// Set test clock.
		start := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, start)

		// Setup a full HTTP server.
		server := &prpctest.Server{}
		pb.RegisterRecorderServer(server, newTestRecorderServer())
		server.Start(ctx)
		defer server.Close()
		client, err := server.NewClient()
		assert.Loosely(t, err, should.BeNil)
		recorder := pb.NewRecorderPRPCClient(client)

		rootInvID := rootinvocations.ID("root-inv-id")
		parentWorkUnitID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "wu-parent",
		}
		prefixedParentWorkUnitID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "wu-parent:parent",
		}
		workUnitID1 := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "wu-parent:wu-new-1",
		}
		workUnitID11 := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "wu-new-11",
		}
		workUnitID2 := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "wu-new-2",
		}

		// Insert a root invocation and the parent work unit.
		rootInv := rootinvocations.NewBuilder(rootInvID).WithRealm("testproject:testrealm").Build()
		parentWu := workunits.NewBuilder(rootInvID, parentWorkUnitID.WorkUnitID).WithState(pb.WorkUnit_ACTIVE).Build()
		parentPrefixedWu := workunits.NewBuilder(rootInvID, prefixedParentWorkUnitID.WorkUnitID).WithState(pb.WorkUnit_ACTIVE).Build()
		testutil.MustApply(ctx, t, insert.RootInvocationWithRootWorkUnit(rootInv)...)
		testutil.MustApply(ctx, t, insert.WorkUnit(parentWu)...)
		testutil.MustApply(ctx, t, insert.WorkUnit(parentPrefixedWu)...)

		// Generate an update token for the parent work unit.
		parentUpdateToken, err := generateWorkUnitUpdateToken(ctx, parentWorkUnitID)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, parentUpdateToken))

		// A basic valid request.
		// parent
		//  |---wu-parent:wu-new-1
		//  |          |--- wu-new-11
		//  |---wu-new-2
		req := &pb.BatchCreateWorkUnitsRequest{
			RequestId: "test-request-id",
			Requests: []*pb.CreateWorkUnitRequest{
				{
					Parent:     parentWorkUnitID.Name(),
					WorkUnitId: workUnitID1.WorkUnitID,
					WorkUnit:   &pb.WorkUnit{Realm: "testproject:testrealm"},
					RequestId:  "test-request-id",
				},
				{
					Parent:     workUnitID1.Name(),
					WorkUnitId: workUnitID11.WorkUnitID,
					WorkUnit:   &pb.WorkUnit{Realm: "testproject:testrealm"},
				},
				{
					Parent:     parentWorkUnitID.Name(),
					WorkUnitId: workUnitID2.WorkUnitID,
					WorkUnit:   &pb.WorkUnit{Realm: "testproject:testrealm"},
				},
			},
		}

		// The validation on this method is fairly involved. To ensure no cases are missed,
		// we do not test the main validation methods separately but do exhaustive testing on the
		// integrated RPC.

		t.Run("request validation", func(t *ftt.Test) {
			t.Run("no requests", func(t *ftt.Test) {
				req.Requests = nil
				_, err := recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("requests: must have at least one request"))
			})
			t.Run("too many requests", func(t *ftt.Test) {
				req.Requests = make([]*pb.CreateWorkUnitRequest, 501)
				_, err := recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("requests: the number of requests in the batch (501) exceeds 500"))
			})
			t.Run("total size of requests is too large", func(t *ftt.Test) {
				req.Requests[0].Parent = strings.Repeat("a", pbutil.MaxBatchRequestSize)
				_, err := recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("requests: the size of all requests is too large"))
			})
			t.Run("sub-requests", func(t *ftt.Test) {
				t.Run("parent", func(t *ftt.Test) {
					t.Run("unspecified", func(t *ftt.Test) {
						req.Requests[1].Parent = ""
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[1]: parent: unspecified"))
					})
					t.Run("invalid", func(t *ftt.Test) {
						req.Requests[1].Parent = "\x00"
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[1]: parent: does not match pattern"))
					})
					t.Run("parent in different root invocation", func(t *ftt.Test) {
						req.Requests[1].Parent = "rootInvocations/another-root/workUnits/root"
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[1]: parent: all requests must be for the same root invocation"))
					})
					t.Run("parent cycle", func(t *ftt.Test) {
						t.Run("cycle of length 1", func(t *ftt.Test) {
							// Requests[0] -> Requests[0]
							req.Requests[0].Parent = workUnitID1.Name()
							_, err := recorder.BatchCreateWorkUnits(ctx, req)
							assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike("requests[0]: parent: cannot refer to the work unit created in requests[0]"))
						})
						t.Run("cycle of length 2", func(t *ftt.Test) {
							// Requests[1] -> Requests[2] -> Requests[1]
							req.Requests[1].Parent = workUnitID2.Name()
							req.Requests[2].Parent = workUnitID11.Name()
							_, err := recorder.BatchCreateWorkUnits(ctx, req)
							assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike("requests[1]: parent: cannot refer to work unit created in the later request requests[2], please order requests to match expected creation order"))
						})
					})
				})
				t.Run("work unit id", func(t *ftt.Test) {
					t.Run("empty", func(t *ftt.Test) {
						req.Requests[1].WorkUnitId = ""
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: work_unit_id: unspecified`))
					})
					t.Run("invalid", func(t *ftt.Test) {
						req.Requests[1].WorkUnitId = "\x00"
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: work_unit_id: does not match pattern`))
					})
					t.Run("duplicated work unit id", func(t *ftt.Test) {
						req.Requests[1].WorkUnitId = req.Requests[0].WorkUnitId
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: work_unit_id: duplicates work unit id "wu-parent:wu-new-1" from requests[0]`))
					})
					t.Run("reserved", func(t *ftt.Test) {
						req.Requests[1].WorkUnitId = "build-1234567890"
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run("not prefixed", func(t *ftt.Test) {
						req.Requests[1].WorkUnitId = "u-my-work-unit-id"
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run("prefixed, in parent is not prefixed", func(t *ftt.Test) {
						t.Run("valid", func(t *ftt.Test) {
							req.Requests[1].WorkUnitId = "wu-parent:child"
							req.Requests[1].Parent = parentWorkUnitID.Name()
							_, err := recorder.BatchCreateWorkUnits(ctx, req)
							assert.Loosely(t, err, should.BeNil)
						})
						t.Run("invalid", func(t *ftt.Test) {
							req.Requests[1].WorkUnitId = "otherworkunit:child"
							req.Requests[1].Parent = parentWorkUnitID.Name()
							_, err := recorder.BatchCreateWorkUnits(ctx, req)
							assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: work_unit_id: work unit ID prefix "otherworkunit" must match parent work unit ID "wu-parent"`))
						})
					})
					t.Run("prefixed, in parent that is prefixed", func(t *ftt.Test) {
						t.Run("valid", func(t *ftt.Test) {
							req.Requests[1].WorkUnitId = "wu-parent:child"
							req.Requests[1].Parent = prefixedParentWorkUnitID.Name()
							_, err := recorder.BatchCreateWorkUnits(ctx, req)
							assert.Loosely(t, err, should.BeNil)
						})
						t.Run("invalid", func(t *ftt.Test) {
							req.Requests[1].WorkUnitId = "otherworkunit:child"
							req.Requests[1].Parent = prefixedParentWorkUnitID.Name()
							_, err := recorder.BatchCreateWorkUnits(ctx, req)
							assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[1]: work_unit_id: work unit ID prefix "otherworkunit" must match parent work unit ID prefix "wu-parent"`))
						})
					})
				})
				t.Run("work unit", func(t *ftt.Test) {
					t.Run("unspecified", func(t *ftt.Test) {
						req.Requests[1].WorkUnit = nil
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[1]: work_unit: unspecified"))
					})
					t.Run("realm", func(t *ftt.Test) {
						t.Run("unspecified", func(t *ftt.Test) {
							req.Requests[1].WorkUnit.Realm = ""
							_, err := recorder.BatchCreateWorkUnits(ctx, req)
							assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike("requests[1]: work_unit: realm: unspecified"))
						})
						t.Run("invalid", func(t *ftt.Test) {
							req.Requests[1].WorkUnit.Realm = "invalid:"
							_, err := recorder.BatchCreateWorkUnits(ctx, req)
							assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike("requests[1]: work_unit: realm: bad global realm name"))
						})
					})
					t.Run("other invalid", func(t *ftt.Test) {
						// validateCreateWorkUnitRequest has its own exhaustive test cases,
						// simply check that it is called.
						req.Requests[1].WorkUnit.State = pb.WorkUnit_FINALIZED
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[1]: work_unit: state: cannot be created in the state FINALIZED"))
					})
				})
			})
			t.Run("request_id", func(t *ftt.Test) {
				t.Run("unspecified", func(t *ftt.Test) {
					req.RequestId = ""
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("request_id: unspecified"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RequestId = "😃"
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("request_id: does not match"))
				})
				t.Run("mismatched child request id", func(t *ftt.Test) {
					req.Requests[1].RequestId = "another-request-id"
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("requests[1]: request_id: inconsistent with top-level request_id"))
				})
				t.Run("matched child request id", func(t *ftt.Test) {
					req.RequestId = "test-request-id"
					req.Requests[1].RequestId = "test-request-id"

					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
			})
		})

		t.Run("request authorization", func(t *ftt.Test) {
			t.Run("basic (same-realm) creation", func(t *ftt.Test) {
				t.Run("with update token", func(t *ftt.Test) {
					t.Run("allowed", func(t *ftt.Test) {
						ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, parentUpdateToken))
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run("requests requiring different update tokens", func(t *ftt.Test) {
						req.Requests[1].Parent = "rootInvocations/root-inv-id/workUnits/other-work-unit"
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: parent "rootInvocations/root-inv-id/workUnits/other-work-unit" requires a different update token to requests[0]'s "parent" "rootInvocations/root-inv-id/workUnits/wu-parent", but this RPC only accepts one update token`))
					})
				})
				t.Run("with inclusion token", func(t *ftt.Test) {
					inclusionToken, err := generateWorkUnitInclusionToken(ctx, parentWorkUnitID, "testproject:testrealm")
					assert.Loosely(t, err, should.BeNil)
					ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.InclusionTokenMetadataKey, inclusionToken))

					// Use the same parent on all requests, inclusion tokens do not extend to inclusions
					// in other work units sharing the same prefix.
					for _, r := range req.Requests {
						r.Parent = parentWorkUnitID.Name()
					}

					t.Run("allowed", func(t *ftt.Test) {
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run("parent requires different inclusion token", func(t *ftt.Test) {
						req.Requests[1].Parent = prefixedParentWorkUnitID.Name()
						_, err = recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: parent "rootInvocations/root-inv-id/workUnits/wu-parent:parent" requires a different inclusion token to requests[0].parent "rootInvocations/root-inv-id/workUnits/wu-parent", but this RPC only accepts one inclusion token`))
					})
					t.Run("disallowed with inclusion token for wrong realm", func(t *ftt.Test) {
						inclusionToken, err := generateWorkUnitInclusionToken(ctx, parentWorkUnitID, "testproject:otherrealm")
						assert.Loosely(t, err, should.BeNil)
						ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.InclusionTokenMetadataKey, inclusionToken))

						_, err = recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
						assert.Loosely(t, err, should.ErrLike(`requests[0]: work_unit: realm: got realm "testproject:testrealm" but inclusion token only authorizes the source to include realm "testproject:otherrealm"`))
					})
				})
			})
			t.Run("cross-realm creation", func(t *ftt.Test) {
				req.Requests[1].WorkUnit.Realm = "testproject:otherrealm"

				t.Run("with update token", func(t *ftt.Test) {
					// Provide the permission required.
					authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
						Realm:      "testproject:otherrealm",
						Permission: permCreateWorkUnit,
					})
					authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
						Realm:      "testproject:otherrealm",
						Permission: permIncludeWorkUnit,
					})
					t.Run("allowed with required permissions", func(t *ftt.Test) {
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run("disallowed without create work unit permission", func(t *ftt.Test) {
						authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permCreateWorkUnit)
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: caller does not have permission "resultdb.workUnits.create" in realm "testproject:otherrealm"`))
					})
					t.Run("disallowed without include work unit permission", func(t *ftt.Test) {
						authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permIncludeWorkUnit)
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: caller does not have permission "resultdb.workUnits.include" in realm "testproject:otherrealm"`))
					})
				})
				t.Run("with inclusion token", func(t *ftt.Test) {
					// Provide the permission required.
					authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
						Realm:      "testproject:otherrealm",
						Permission: permCreateWorkUnit,
					})

					inclusionToken, err := generateWorkUnitInclusionToken(ctx, parentWorkUnitID, "testproject:otherrealm")
					assert.Loosely(t, err, should.BeNil)
					ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.InclusionTokenMetadataKey, inclusionToken))

					for _, r := range req.Requests {
						// Use the same realm and parent on all requests, inclusion tokens do not extend
						// to multiple parents sharing the same prefix or realms.
						r.WorkUnit.Realm = "testproject:otherrealm"
						r.Parent = parentWorkUnitID.Name()
					}

					t.Run("allowed with required permissions", func(t *ftt.Test) {
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run("disallowed without create work unit permission", func(t *ftt.Test) {
						authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permCreateWorkUnit)
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
						assert.Loosely(t, err, should.ErrLike(`requests[0]: caller does not have permission "resultdb.workUnits.create" in realm "testproject:otherrealm"`))
					})
					t.Run("disallowed with inclusion token for wrong realm", func(t *ftt.Test) {
						req.Requests[1].WorkUnit.Realm = "testproject:thirdrealm"

						_, err = recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: work_unit: realm: got realm "testproject:thirdrealm" but inclusion token only authorizes the source to include realm "testproject:otherrealm"`))
					})
				})
			})
			t.Run("reserved id", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permCreateWorkUnitWithReservedID)
				req.Requests[1].WorkUnitId = "build-8765432100"

				t.Run("disallowed without realm permission", func(t *ftt.Test) {
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.Loosely(t, err, should.ErrLike(`requests[1]: work_unit_id: only work units created by trusted systems may have id not starting with "u-"`))
				})
				t.Run("allowed with realm permission", func(t *ftt.Test) {
					authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
						Realm: "testproject:@root", Permission: permCreateWorkUnitWithReservedID,
					})
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("allowed with trusted group", func(t *ftt.Test) {
					authState.IdentityGroups = []string{trustedCreatorGroup}
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
			})
			t.Run("producer resource", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permSetWorkUnitProducerResource)
				req.Requests[1].WorkUnit.ProducerResource = "//builds.example.com/builds/1"
				t.Run("disallowed without permission", func(t *ftt.Test) {
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.Loosely(t, err, should.ErrLike(`requests[1]: work_unit: producer_resource: only work units created by trusted system may have a populated producer_resource field`))
				})
				t.Run("allowed with realm permission", func(t *ftt.Test) {
					authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
						Realm: "testproject:@root", Permission: permSetWorkUnitProducerResource,
					})
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("allowed with trusted group", func(t *ftt.Test) {
					authState.IdentityGroups = []string{trustedCreatorGroup}
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
			})
			t.Run("inclusion or update token is validated", func(t *ftt.Test) {
				t.Run("invalid update token", func(t *ftt.Test) {
					ctx := metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.That(t, err, should.ErrLike(`invalid update token`))
				})
				t.Run("invalid inclusion token", func(t *ftt.Test) {
					// Use the same parent on all requests as otherwise the request cannot be
					// authorized by an inclusion token (however valid it is).
					for _, r := range req.Requests {
						r.Parent = parentWorkUnitID.Name()
					}

					ctx := metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.InclusionTokenMetadataKey, "invalid-token"))
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.That(t, err, should.ErrLike(`invalid inclusion token`))
				})
				t.Run("missing update and inclusion token", func(t *ftt.Test) {
					// Other test cases are covered by extractInclusionOrUpdateToken.
					ctx := metadata.NewOutgoingContext(ctx, metadata.MD{})
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.Unauthenticated))
					assert.That(t, err, should.ErrLike(`expected either update-token or inclusion-token metadata value in the request`))
				})
			})
		})

		t.Run("parent", func(t *ftt.Test) {
			t.Run("parent not active", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, spanutil.UpdateMap("WorkUnits", map[string]any{
					"RootInvocationShardId": parentWorkUnitID.RootInvocationShardID(),
					"WorkUnitId":            parentWorkUnitID.WorkUnitID,
					"State":                 pb.WorkUnit_FINALIZING,
				}))

				_, err = recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
				assert.That(t, err, should.ErrLike(`parent "rootInvocations/root-inv-id/workUnits/wu-parent" is not active`))
			})

			t.Run("parent does not exist and not created in the request", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, spanner.Delete("WorkUnits", parentWorkUnitID.Key()))

				_, err = recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.That(t, err, should.ErrLike(`"rootInvocations/root-inv-id/workUnits/wu-parent" not found`))
			})
		})
		t.Run("already exists", func(t *ftt.Test) {
			t.Run("exist with different request id", func(t *ftt.Test) {
				// Create just one of the work units to be created.
				wu := workunits.NewBuilder(rootInvID, workUnitID11.WorkUnitID).WithCreateRequestID("different-request-id").Build()
				testutil.MustApply(ctx, t, insert.WorkUnit(wu)...)

				_, err := recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.AlreadyExists))
				assert.That(t, err, should.ErrLike("\"rootInvocations/root-inv-id/workUnits/wu-new-11\" already exists with different requestID or creator"))
			})
			t.Run("partial exist with the same request id", func(t *ftt.Test) {
				// Create all work units to be created, but with a different request ID.
				wu := workunits.NewBuilder(rootInvID, workUnitID2.WorkUnitID).WithCreateRequestID("test-request-id").WithCreatedBy("user:someone@example.com").Build()
				testutil.MustApply(ctx, t, insert.WorkUnit(wu)...)

				_, err := recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.AlreadyExists))
				// The error message depends on which row is read first, so we just check for the general error string.
				assert.That(t, err, should.ErrLike("some work units already exist (eg. \"rootInvocations/root-inv-id/workUnits/wu-new-2\")"))
			})
		})

		t.Run("create is idempotent", func(t *ftt.Test) {
			res1, err := recorder.BatchCreateWorkUnits(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			// Send the exact same request again.
			res2, err := recorder.BatchCreateWorkUnits(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, res2, should.Match(res1))

			// Check the database has the expected number of entries.
			readCtx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			rows, err := workunits.ReadBatch(readCtx, []workunits.ID{workUnitID1, workUnitID11, workUnitID2}, workunits.AllFields)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rows, should.HaveLength(3))
		})

		t.Run("end to end success", func(t *ftt.Test) {
			wuProperties := &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"@type": structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
					"key":   structpb.NewStringValue("workunit"),
				},
			}
			instructions := testutil.TestInstructions()
			extendedProperties := map[string]*structpb.Struct{
				"mykey": {
					Fields: map[string]*structpb.Value{
						"@type":       structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
						"child_key_1": structpb.NewStringValue("child_value_1"),
					},
				},
			}

			// Populare Requests[0] with all fields and Requests[1] and [2] with minimal fields
			// to maximise test coverage.
			req.Requests[0].WorkUnit = &pb.WorkUnit{
				Realm: "testproject:testrealm",
				State: pb.WorkUnit_FINALIZING,
				ModuleId: &pb.ModuleIdentifier{
					ModuleName:    "mymodule",
					ModuleScheme:  "gtest",
					ModuleVariant: pbutil.Variant("k", "v"),
				},
				ProducerResource:   "//producer.example.com/builds/123",
				Tags:               pbutil.StringPairs("e2e_key", "e2e_value"),
				Properties:         wuProperties,
				ExtendedProperties: extendedProperties,
				Instructions:       instructions,
			}

			// Expected work units in the response.
			expectedWU1 := proto.Clone(req.Requests[0].WorkUnit).(*pb.WorkUnit)
			proto.Merge(expectedWU1, &pb.WorkUnit{
				Name:           workUnitID1.Name(),
				Parent:         parentWorkUnitID.Name(),
				WorkUnitId:     workUnitID1.WorkUnitID,
				Creator:        "user:someone@example.com",
				Deadline:       timestamppb.New(start.Add(defaultDeadlineDuration)),
				ChildWorkUnits: []string{workUnitID11.Name()},
			})
			expectedWU1.Instructions = instructionutil.InstructionsWithNames(instructions, workUnitID1.Name())
			pbutil.PopulateModuleIdentifierHashes(expectedWU1.ModuleId)

			expectedWU11 := proto.Clone(req.Requests[1].WorkUnit).(*pb.WorkUnit)
			proto.Merge(expectedWU11, &pb.WorkUnit{
				Name:       workUnitID11.Name(),
				Parent:     workUnitID1.Name(),
				WorkUnitId: workUnitID11.WorkUnitID,
				State:      pb.WorkUnit_ACTIVE, // Default state is ACTIVE.
				Creator:    "user:someone@example.com",
				Deadline:   timestamppb.New(start.Add(defaultDeadlineDuration)),
			})

			expectedWU2 := proto.Clone(req.Requests[2].WorkUnit).(*pb.WorkUnit)
			proto.Merge(expectedWU2, &pb.WorkUnit{
				Name:       workUnitID2.Name(),
				Parent:     parentWorkUnitID.Name(),
				WorkUnitId: workUnitID2.WorkUnitID,
				State:      pb.WorkUnit_ACTIVE, // Default state is ACTIVE.
				Creator:    "user:someone@example.com",
				Deadline:   timestamppb.New(start.Add(defaultDeadlineDuration)),
			})

			expectWURow1 := &workunits.WorkUnitRow{
				ID:                workUnitID1,
				ParentWorkUnitID:  spanner.NullString{Valid: true, StringVal: parentWorkUnitID.WorkUnitID},
				State:             pb.WorkUnit_FINALIZING,
				Realm:             "testproject:testrealm",
				CreatedBy:         "user:someone@example.com",
				FinalizeStartTime: spanner.NullTime{},
				FinalizeTime:      spanner.NullTime{},
				Deadline:          start.Add(defaultDeadlineDuration),
				CreateRequestID:   "test-request-id",
				ModuleID: &pb.ModuleIdentifier{
					ModuleName:        "mymodule",
					ModuleScheme:      "gtest",
					ModuleVariant:     pbutil.Variant("k", "v"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("k", "v")),
				},
				ProducerResource:   "//producer.example.com/builds/123",
				Tags:               pbutil.StringPairs("e2e_key", "e2e_value"),
				Properties:         wuProperties,
				Instructions:       instructionutil.InstructionsWithNames(instructions, workUnitID1.Name()),
				ExtendedProperties: extendedProperties,
				ChildWorkUnits:     []workunits.ID{workUnitID11},
			}

			expectWURow11 := &workunits.WorkUnitRow{
				ID:                workUnitID11,
				ParentWorkUnitID:  spanner.NullString{Valid: true, StringVal: workUnitID1.WorkUnitID},
				State:             pb.WorkUnit_ACTIVE,
				Realm:             "testproject:testrealm",
				CreatedBy:         "user:someone@example.com",
				FinalizeStartTime: spanner.NullTime{},
				FinalizeTime:      spanner.NullTime{},
				Deadline:          start.Add(defaultDeadlineDuration),
				CreateRequestID:   "test-request-id",
			}

			expectWURow2 := &workunits.WorkUnitRow{
				ID:                workUnitID2,
				ParentWorkUnitID:  spanner.NullString{Valid: true, StringVal: parentWorkUnitID.WorkUnitID},
				State:             pb.WorkUnit_ACTIVE,
				Realm:             "testproject:testrealm",
				CreatedBy:         "user:someone@example.com",
				FinalizeStartTime: spanner.NullTime{},
				FinalizeTime:      spanner.NullTime{},
				Deadline:          start.Add(defaultDeadlineDuration),
				CreateRequestID:   "test-request-id",
			}

			// Expected update token
			workUnitID11ExpectedUpdateToken, err := generateWorkUnitUpdateToken(ctx, workUnitID11)
			assert.Loosely(t, err, should.BeNil)
			workUnitID2ExpectedUpdateToken, err := generateWorkUnitUpdateToken(ctx, workUnitID2)
			assert.Loosely(t, err, should.BeNil)

			res, err := recorder.BatchCreateWorkUnits(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			createTime := res.WorkUnits[0].CreateTime.AsTime()
			expectedWU1.CreateTime = timestamppb.New(createTime)
			expectedWU1.LastUpdated = timestamppb.New(createTime)
			expectedWU1.FinalizeStartTime = timestamppb.New(createTime)
			expectedWU1.Etag = fmt.Sprintf(`W/"+f/%s"`, createTime.UTC().Format(time.RFC3339Nano))
			expectedWU11.CreateTime = timestamppb.New(createTime)
			expectedWU11.LastUpdated = timestamppb.New(createTime)
			expectedWU11.Etag = fmt.Sprintf(`W/"+f/%s"`, createTime.UTC().Format(time.RFC3339Nano))
			expectedWU2.CreateTime = timestamppb.New(createTime)
			expectedWU2.LastUpdated = timestamppb.New(createTime)
			expectedWU2.Etag = fmt.Sprintf(`W/"+f/%s"`, createTime.UTC().Format(time.RFC3339Nano))
			assert.That(t, res, should.Match(
				&pb.BatchCreateWorkUnitsResponse{
					WorkUnits: []*pb.WorkUnit{expectedWU1, expectedWU11, expectedWU2},
					UpdateTokens: []string{
						parentUpdateToken,
						workUnitID11ExpectedUpdateToken,
						workUnitID2ExpectedUpdateToken,
					},
				},
			))
			assert.Loosely(t, res.WorkUnits[0].FinalizeStartTime, should.NotBeNil)

			// Check the database.
			readCtx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			row, err := workunits.Read(readCtx, workUnitID1, workunits.AllFields)
			assert.Loosely(t, err, should.BeNil)
			expectWURow1.SecondaryIndexShardID = row.SecondaryIndexShardID
			expectWURow1.CreateTime = createTime
			expectWURow1.LastUpdated = createTime
			expectWURow1.FinalizeStartTime = spanner.NullTime{Time: createTime, Valid: true}
			assert.That(t, row, should.Match(expectWURow1))

			row, err = workunits.Read(readCtx, workUnitID2, workunits.AllFields)
			assert.Loosely(t, err, should.BeNil)
			expectWURow2.SecondaryIndexShardID = row.SecondaryIndexShardID
			expectWURow2.CreateTime = createTime
			expectWURow2.LastUpdated = createTime
			expectWURow2.Tags = []*pb.StringPair{}
			assert.That(t, row, should.Match(expectWURow2))

			row, err = workunits.Read(readCtx, workUnitID11, workunits.AllFields)
			assert.Loosely(t, err, should.BeNil)
			expectWURow11.SecondaryIndexShardID = row.SecondaryIndexShardID
			expectWURow11.CreateTime = createTime
			expectWURow11.LastUpdated = createTime
			expectWURow11.Tags = []*pb.StringPair{}
			assert.That(t, row, should.Match(expectWURow11))

			// Check the legacy invocation is inserted.
			legacyInvs, err := invocations.ReadBatch(readCtx, invocations.NewIDSet(workUnitID1.LegacyInvocationID(), workUnitID11.LegacyInvocationID(), workUnitID2.LegacyInvocationID()), invocations.AllFields)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, legacyInvs, should.HaveLength(3))

			// Check inclusion is added to IncludedInvocations.
			includedIDs, err := invocations.ReadIncluded(readCtx, parentWorkUnitID.LegacyInvocationID())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, includedIDs, should.HaveLength(2))
			assert.That(t, includedIDs.Has(workUnitID1.LegacyInvocationID()), should.BeTrue)
			assert.That(t, includedIDs.Has(workUnitID2.LegacyInvocationID()), should.BeTrue)

			includedIDs, err = invocations.ReadIncluded(readCtx, workUnitID1.LegacyInvocationID())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, includedIDs, should.HaveLength(1))
			assert.That(t, includedIDs.Has(workUnitID11.LegacyInvocationID()), should.BeTrue)
		})
	})
}
